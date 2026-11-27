import { describe, expect, it } from "vitest";
import {
  asSessionId,
  mutationSettlementIsUnknown,
  type FlameClient,
  type PluginCommandChange,
} from "@flame/runtime-client";
import {
  HTTP_HEADERS,
  type CallPluginRequest,
  type CallPluginResponse,
} from "@flame/runtime-client/wire";
import { PendingPluginCommands } from "@/features/plugins/adapters/pendingPluginCommands";
import { METHODS } from "../../../../../plugins/schedules/component/methods";
import { bindScheduleOperations } from "../../../../../plugins/schedules/component/scheduleCommands";
import { type RunReference } from "../../../../../plugins/schedules/package/io.github.tangerg.flame/contracts/methods.generated";
import { useRuntimeFixture } from "./fixture";
import { within } from "./observation";

function observeResolutions(client: FlameClient) {
  const resolutions: Array<{
    change: Extract<PluginCommandChange, { type: "resolved" }>;
    retained: boolean;
  }> = [];
  const stop = client.plugins.subscribeCommands((change) => {
    if (change.type !== "resolved") return;
    resolutions.push({
      change,
      retained: client.plugins.commands().some(
        ({ idempotencyKey }) => idempotencyKey === change.idempotencyKey,
      ),
    });
  });
  return { resolutions, stop };
}

describe("Schedules commands over Runtime HTTP", () => {
  const fixture = useRuntimeFixture("schedules");

  const bindView = (runtime: FlameClient, signal: AbortSignal) => {
    const installed = fixture.managedOwners.get("schedules")!;
    const component = installed.selected.components.find(({ id }) => id === "schedules")!;
    const contribution = installed.selected.contributions.find(
      ({ id }) => id === component.contribution,
    )!;
    return bindScheduleOperations({
      binding: {
        installationId: installed.id,
        digest: installed.selected.digest,
        contribution: contribution.id,
        contract: contribution.contract,
      },
      runtime,
      signal,
      track: (operation) => operation(signal),
    });
  };

  it("keeps the original manual run across view withdrawal and reply loss", async () => {
    const committed = Promise.withResolvers<RunReference>();
    const dropReply = Promise.withResolvers<void>();
    const delivered: (string | null)[] = [];
    const recovered: string[] = [];
    const client = fixture.createRuntimeClient(async (url, init) => {
      const headers = new Headers(init?.headers);
      const method = headers.get(HTTP_HEADERS.method);
      if (method === "plugins.getCommand") {
        const { params } = JSON.parse(String(init?.body)) as { params: { key: string } };
        recovered.push(params.key);
      }
      if (method !== "plugins.call") return fetch(url, init);
      const { params } = JSON.parse(String(init?.body)) as { params: CallPluginRequest };
      if (params.route.method !== METHODS.runNow) return fetch(url, init);
      delivered.push(headers.get(HTTP_HEADERS.idempotencyKey));
      if (delivered.length !== 1) return fetch(url, init);
      const response = await fetch(url, init);
      const reply = (await response.clone().json()) as { result?: CallPluginResponse };
      if (!reply.result?.output) {
        committed.reject(
          new Error(`Runtime did not accept the manual Run: ${JSON.stringify(reply)}`),
        );
        return response;
      }
      committed.resolve(JSON.parse(reply.result.output) as RunReference);
      await dropReply.promise;
      throw new TypeError("injected loss of the original Schedule response");
    });
    const { resolutions, stop } = observeResolutions(client);
    const originalLifetime = new AbortController();
    const replacementLifetime = new AbortController();
    let original: Promise<unknown> | undefined;
    try {
      const first = bindView(client, originalLifetime.signal);
      const schedule = await first.submit(METHODS.create, {
        title: "Manual run receipt",
        instructions: "Report the manual run receipt.",
        cron: "0 0 1 1 *",
        timeZone: "UTC",
        cwd: fixture.root,
        enabled: false,
        modelSelection: { provider: "", model: "", reasoningEffort: "" },
      });
      if (!schedule) throw new Error("the initial Schedule was not submitted");
      const input = { id: schedule.id };
      const starting = first.submit(METHODS.runNow, input);
      if (!starting) throw new Error("the original Schedule run was not submitted");
      original = starting.catch((error: unknown) => error);
      const receipt = await within(
        Promise.race([
          committed.promise,
          starting.then(() => {
            throw new Error("the original Schedule response bypassed the hold");
          }),
        ]),
        "the original Schedule run commit",
      );
      const second = bindView(client, replacementLifetime.signal);
      expect(second.originalCommand()).toMatchObject({
        state: "active",
        method: METHODS.runNow,
        idempotencyKey: delivered[0],
      });
      expect(second.submit(METHODS.runNow, input)).toBeUndefined();
      expect(delivered).toHaveLength(1);

      originalLifetime.abort();
      dropReply.resolve();
      expect(mutationSettlementIsUnknown(await original)).toBe(true);
      expect(second.submit(METHODS.runNow, input)).toBeUndefined();
      const command = second.originalCommand();
      if (command?.state !== "recoverable") throw new Error("the original Schedule command is unavailable");
      expect(command).toMatchObject({ method: METHODS.runNow, idempotencyKey: delivered[0] });
      expect(resolutions).toEqual([]);
      await expect(command.recover()).resolves.toEqual(receipt);
      expect(recovered).toEqual([command.idempotencyKey]);
      expect(delivered).toEqual([command.idempotencyKey, command.idempotencyKey]);
      expect(second.originalCommand()).toBeUndefined();
      expect(resolutions).toEqual([{
        change: { type: "resolved", idempotencyKey: command.idempotencyKey, resolution: { type: "confirmed" } },
        retained: false,
      }]);

      const sessionId = asSessionId(receipt.sessionId);
      await expect(fixture.client.sessions.get(sessionId)).resolves.toMatchObject({
        id: sessionId,
      });
      const runs = await fixture.client.runs.list({ sessionId });
      expect(runs.data.map(({ id }) => id)).toEqual([receipt.runId]);
    } finally {
      stop();
      originalLifetime.abort();
      replacementLifetime.abort();
      dropReply.resolve();
      await original;
      await client.close();
    }
  }, 30_000);

  it("retires a captured Schedule command before its first HTTP delivery", async () => {
    const pendingDelivery = Promise.withResolvers<{
      key: string;
      send: () => Promise<Response>;
    }>();
    const originalLifetime = new AbortController();
    const replacementLifetime = new AbortController();
    let holdOriginal = true;
    const client = fixture.createRuntimeClient((url, init) => {
      const headers = new Headers(init?.headers);
      if (!holdOriginal || headers.get(HTTP_HEADERS.method) !== "plugins.call")
        return fetch(url, init);
      const { params } = JSON.parse(String(init?.body)) as { params: CallPluginRequest };
      if (params.route.method !== METHODS.create) return fetch(url, init);
      holdOriginal = false;
      const signal = init?.signal;
      if (!signal) throw new Error("the original delivery has no lifetime");
      pendingDelivery.resolve({
        key: headers.get(HTTP_HEADERS.idempotencyKey)!,
        send: () => fetch(url, { ...init, signal: undefined }),
      });
      return new Promise<never>((_, reject) => {
        signal.addEventListener("abort", () => reject(signal.reason), { once: true });
      });
    });
    const { resolutions, stop } = observeResolutions(client);
    let outstanding: Promise<unknown> | undefined;
    try {
      const management = new PendingPluginCommands(
        replacementLifetime.signal,
        client.plugins,
        (operation) => operation(replacementLifetime.signal),
      );
      expect(management.snapshot()?.read).toMatchObject({ type: "ready", commands: [] });
      const first = bindView(client, originalLifetime.signal);
      const draft = {
        title: "Retired before HTTP delivery",
        instructions: "Report the original delivery.",
        cron: "0 0 1 1 *",
        timeZone: "UTC",
        cwd: fixture.root,
        enabled: false,
        modelSelection: { provider: "", model: "", reasoningEffort: "" },
      };
      const creating = first.submit(METHODS.create, draft);
      if (!creating) throw new Error("the original command was not reserved");
      outstanding = creating.catch((error: unknown) => error);
      const delayed = await within(pendingDelivery.promise, "the withheld Schedule request");
      originalLifetime.abort();
      expect(mutationSettlementIsUnknown(await outstanding)).toBe(true);
      expect(management.snapshot()?.read).toMatchObject({
        type: "ready",
        commands: [expect.objectContaining({ idempotencyKey: delayed.key })],
      });

      const original = client.plugins
        .commands()
        .find(({ idempotencyKey }) => idempotencyKey === delayed.key);
      if (original?.state !== "recoverable") throw new Error("the SDK did not retain its undelivered command");
      await original.retire(replacementLifetime.signal);
      expect(resolutions).toEqual([{
        change: { type: "resolved", idempotencyKey: delayed.key, resolution: { type: "retired" } },
        retained: false,
      }]);
      expect(management.snapshot()?.read).toMatchObject({ type: "ready", commands: [] });
      const replacement = bindView(client, replacementLifetime.signal);
      expect(replacement.originalCommand()).toBeUndefined();
      const reply = await delayed.send();
      await expect(reply.json()).resolves.toMatchObject({
        error: { data: { problem: { type: "plugin_command_retired" } } },
      });
      const page = await replacement.query(METHODS.list, { cursor: "" });
      expect(page.rows.some(({ title }) => title === draft.title)).toBe(false);

      const created = await replacement.submit(METHODS.create, draft);
      if (!created) throw new Error("the retired command kept the authoring slot occupied");
      const current = await replacement.query(METHODS.list, { cursor: "" });
      expect(current.rows.filter(({ title }) => title === draft.title).map(({ id }) => id)).toEqual(
        [created.id],
      );
    } finally {
      stop();
      originalLifetime.abort();
      replacementLifetime.abort();
      await outstanding;
      await client.close();
    }
  }, 30_000);

  it("publishes a recovered revision refusal without authorizing a new write", async () => {
    let readingOriginal = false;
    const client = fixture.createRuntimeClient(async (url, init) => {
      const method = new Headers(init?.headers).get(HTTP_HEADERS.method);
      if (method === "plugins.getCommand") readingOriginal = true;
      const response = await fetch(url, init);
      if (readingOriginal || method !== "plugins.call") return response;
      const { params } = JSON.parse(String(init?.body)) as { params: CallPluginRequest };
      if (params.route.method !== METHODS.update) return response;
      await response.arrayBuffer();
      throw new TypeError("injected loss of a stale Schedule update reply");
    });
    const lifetime = new AbortController();
    const { resolutions, stop } = observeResolutions(client);
    try {
      const view = bindView(client, lifetime.signal);
      const schedule = await view.submit(METHODS.create, {
        title: "Original Schedule revision",
        instructions: "Report the Schedule revision.",
        cron: "0 0 1 1 *",
        timeZone: "UTC",
        cwd: fixture.root,
        enabled: false,
        modelSelection: { provider: "", model: "", reasoningEffort: "" },
      });
      if (!schedule) throw new Error("the fixture Schedule was not created");
      await expect(view.submit(METHODS.update, {
        id: schedule.id,
        title: "Conflicting update",
        expectedRevision: 2,
        modelSelection: {},
      })).rejects.toSatisfy(mutationSettlementIsUnknown);
      const original = view.originalCommand();
      if (original?.state !== "recoverable") throw new Error("the update was not retained");
      expect(resolutions).toEqual([]);
      await expect(original.recover()).rejects.toMatchObject({ code: "revision" });
      expect(resolutions).toEqual([{
        change: {
          type: "resolved",
          idempotencyKey: original.idempotencyKey,
          resolution: { type: "refused", code: "revision" },
        },
        retained: false,
      }]);
      const current = await view.query(METHODS.list, { cursor: "" });
      expect(current.rows.find(({ id }) => id === schedule.id)).toEqual(schedule);
    } finally {
      stop();
      lifetime.abort();
      await client.close();
    }
  }, 30_000);
});

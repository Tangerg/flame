import { describe, expect, it } from "vitest";
import {
  asSessionId,
  MutationJournalOwnershipError,
  mutationSettlementIsUnknown,
} from "@flame/runtime-client";
import {
  HTTP_HEADERS,
  type CallPluginRequest,
  type CallPluginResponse,
} from "@flame/runtime-client/wire";
import { METHODS } from "../../../../../plugins/schedules/component/methods";
import { bindScheduleCommands } from "../../../../../plugins/schedules/component/scheduleCommands";
import { type RunReference } from "../../../../../plugins/schedules/package/io.github.tangerg.flame/contracts/methods.generated";
import { useRuntimeFixture } from "./fixture";
import { within } from "./observation";

describe("Schedules commands over Runtime HTTP", () => {
  const fixture = useRuntimeFixture("schedules");

  it("keeps the original manual run across view withdrawal and reply loss", async () => {
    const installed = fixture.managedOwners.get("schedules")!;
    const component = installed.selected.components.find(({ id }) => id === "schedules")!;
    const contribution = installed.selected.contributions.find(
      ({ id }) => id === component.contribution,
    )!;
    const binding = {
      installationId: installed.id,
      digest: installed.selected.digest,
      contribution: contribution.id,
      contract: contribution.contract,
    };
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
    const openView = (signal: AbortSignal) =>
      bindScheduleCommands({
        binding,
        runtime: client,
        signal,
        track: (operation) => operation(signal),
      });
    const originalLifetime = new AbortController();
    const replacementLifetime = new AbortController();
    let original: Promise<unknown> | undefined;
    try {
      const first = openView(originalLifetime.signal);
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
      const second = openView(replacementLifetime.signal);
      expect(second.pendingCommand()).toBeUndefined();
      await expect(second.submit(METHODS.runNow, input)).rejects.toBeInstanceOf(
        MutationJournalOwnershipError,
      );
      expect(delivered).toHaveLength(1);

      originalLifetime.abort();
      dropReply.resolve();
      expect(mutationSettlementIsUnknown(await original)).toBe(true);
      expect(second.submit(METHODS.runNow, input)).toBeUndefined();
      const command = second.pendingCommand();
      expect(command).toMatchObject({ method: METHODS.runNow, idempotencyKey: delivered[0] });
      await expect(command!.recover()).resolves.toEqual(receipt);
      expect(recovered).toEqual([command!.idempotencyKey]);
      expect(delivered).toEqual([command!.idempotencyKey, command!.idempotencyKey]);
      expect(second.pendingCommand()).toBeUndefined();

      const sessionId = asSessionId(receipt.sessionId);
      await expect(fixture.client.sessions.get(sessionId)).resolves.toMatchObject({
        id: sessionId,
      });
      const runs = await fixture.client.runs.list({ sessionId });
      expect(runs.data.map(({ id }) => id)).toEqual([receipt.runId]);
    } finally {
      originalLifetime.abort();
      replacementLifetime.abort();
      dropReply.resolve();
      await original;
      await client.close();
    }
  }, 30_000);
});

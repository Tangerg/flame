import { describe, expect, it, vi } from "vitest";
import { MutationLookupError } from "./mutation";
import { createMethods } from "./methods";
import type { RpcClient, RpcCallOptions } from "./client";
import type { RequestMeta } from "./generated/wire.generated";
import { RpcProtocolError, RpcTransportError } from "./errors";
import {
  createMutationJournal,
  MutationJournalStorageError,
  type MutationJournalStorage,
} from "./mutationJournal";
import { asRunId, asSegmentId } from "./ids";
import discovery from "../../../../runtime/contract/samples/method.discover.resp.json";
import run from "../../../../runtime/contract/samples/runref.full.json";

function fixture() {
  const values = new Map<string, unknown>();
  const storage: MutationJournalStorage = {
    get: (key) => structuredClone(values.get(key)),
    set: (key, value) => void values.set(key, structuredClone(value)),
    remove: (key) => void values.delete(key),
    keys: () => [...values.keys()],
  };
  let now = 1000;
  let namespace = "idp_original";
  const open = (call: RpcClient["call"], requestMeta?: RequestMeta) => {
    const journal = createMutationJournal({
      storage,
      now: () => now,
      scope: () => ({ namespace, retentionSeconds: 60 }),
    });
    const methods = createMethods({ call } as RpcClient, {
      mutationJournal: journal,
      capabilities: () =>
        discovery.capabilities as import("./generated/wire.generated").ServerCapabilities,
      requestMeta: () => requestMeta,
    });
    return { journal, methods };
  };
  return {
    values,
    storage,
    open,
    expire: () => {
      now += 61_000;
    },
    replace: () => {
      namespace = "idp_replaced";
    },
  };
}

const original = {
  runId: asRunId("run_original"),
  expectedSegmentId: asSegmentId("seg_original"),
  input: [
    { type: "text" as const, text: "frozen input" },
    { type: "image" as const, mime: "image/png", data: "aW1hZ2U=" },
  ],
};

describe("cold original steer recovery", () => {
  it("finds a committed original command after client restart without redispatching it", async () => {
    const f = fixture();
    let accepted = false;
    const call = vi.fn(async (method: string) => {
      if (method === "runs.getAcceptance")
        return accepted
          ? {
              type: "accepted",
              sessionId: "ses_1",
              runId: original.runId,
              segmentId: original.expectedSegmentId,
              userItemId: "item_original",
              input: original.input,
              run,
            }
          : { type: "notRecorded", candidateUserItemId: "item_original" };
      accepted = true;
      throw new RpcTransportError("ACK lost after admission");
    });
    const first = f.open(call as RpcClient["call"]);
    const submitted = first.methods.runs.steer(
      original.runId,
      original.expectedSegmentId,
      original.input,
    );
    await expect(submitted).resolves.toEqual({ type: "accepted", userItemId: "item_original" });
    expect(call.mock.calls.filter(([method]) => method === "runs.steer")).toHaveLength(1);
    first.journal.dispose();

    // Model a process dying while the first response is still unavailable.
    accepted = false;
    const lost = f.open(
      vi.fn(async (method: string) => {
        if (method === "runs.getAcceptance")
          return { type: "notRecorded", candidateUserItemId: "item_original" };
        throw new RpcTransportError("process died before response");
      }) as RpcClient["call"],
    );
    const input = structuredClone(original.input);
    const pending = lost.methods.runs.steer(original.runId, original.expectedSegmentId, input);
    await expect(pending).rejects.toBeInstanceOf(RpcTransportError);
    input[0] = { type: "text", text: "new draft" };
    const stored = [...f.values.values()];
    lost.journal.dispose();
    accepted = true;
    const recovered = f.open(call as RpcClient["call"]);
    call.mockClear();
    f.expire();
    const [command] = await recovered.methods.runs.recoverCommands();
    if (command?.operation !== "steer") throw new Error("expected original steer");
    expect(command.idempotencyKey).toBe(pending.idempotencyKey);
    command!.params.input[0] = { type: "text", text: "modified public descriptor" };
    expect(command!.observation).toMatchObject({
      type: "known",
      acceptance: { userItemId: "item_original", input: original.input },
    });
    expect(call.mock.calls.map(([method]) => method)).toEqual(["runs.getAcceptance"]);
    expect(f.values.size).toBe(0);
    await expect(command!.retry()).resolves.toEqual({
      type: "accepted",
      userItemId: "item_original",
    });
    expect(call.mock.calls.every(([method]) => method === "runs.getAcceptance")).toBe(true);
    expect(stored).toHaveLength(1);
    recovered.journal.dispose();
  });

  it.each(["expired", "replaced", "lookup failure"])(
    "keeps %s evidence without granting a new delivery",
    async (condition) => {
      const f = fixture();
      const call = vi.fn(async (method: string) => {
        if (method === "runs.getAcceptance")
          return { type: "notRecorded", candidateUserItemId: "item_original" };
        throw new RpcTransportError("lost response");
      });
      const first = f.open(call as RpcClient["call"]);
      const pending = first.methods.runs.steer(
        original.runId,
        original.expectedSegmentId,
        original.input,
      );
      await expect(pending).rejects.toBeInstanceOf(RpcTransportError);
      const saved = structuredClone([...f.values.values()]);
      first.journal.dispose();
      if (condition === "expired") f.expire();
      if (condition === "replaced") f.replace();
      const second = f.open(call as RpcClient["call"]);
      call.mockClear();
      if (condition === "lookup failure")
        call.mockRejectedValue(new RpcTransportError("query unavailable"));
      const [command] = await second.methods.runs.recoverCommands();
      expect(command?.operation === "steer" && command.userItemId).toBe("item_original");
      expect(command!.observation.type).toBe(condition === "lookup failure" ? "unknown" : "known");
      await expect(command!.retry()).rejects.toBeDefined();
      expect(call.mock.calls.every(([method]) => method === "runs.getAcceptance")).toBe(true);
      expect([...f.values.values()]).toEqual(saved);
      second.journal.dispose();
    },
  );

  it("retries only the persisted body and Segment under the same key after a pure not-recorded query", async () => {
    const f = fixture();
    const requestMeta: RequestMeta = {
      clientCapabilities: { features: { subagents: { enabled: true } } },
    };
    const originalMeta = structuredClone(requestMeta);
    const call = vi.fn(async (method: string) => {
      if (method === "runs.getAcceptance")
        return { type: "notRecorded", candidateUserItemId: "item_original" };
      throw new RpcTransportError("lost");
    });
    const first = f.open(call as RpcClient["call"], requestMeta);
    const pending = first.methods.runs.steer(
      original.runId,
      original.expectedSegmentId,
      original.input,
    );
    await expect(pending).rejects.toBeInstanceOf(RpcTransportError);
    first.journal.dispose();
    requestMeta.clientCapabilities!.features!.subagents!.enabled = false;
    const replay = vi.fn(async (method: string, _params: unknown, _options?: RpcCallOptions) =>
      method === "runs.getAcceptance"
        ? { type: "notRecorded", candidateUserItemId: "item_original" }
        : { userItemId: "item_original" },
    );
    const second = f.open(replay as RpcClient["call"], requestMeta);
    const [command] = await second.methods.runs.recoverCommands();
    expect(command!.observation).toEqual({
      type: "known",
      acceptance: { type: "notRecorded", candidateUserItemId: "item_original" },
    });
    expect(replay).toHaveBeenCalledTimes(1);
    if (command?.operation !== "steer") throw new Error("expected original steer");
    command.params.expectedSegmentId = asSegmentId("seg_new");
    await expect(command!.retry()).resolves.toEqual({
      type: "accepted",
      userItemId: "item_original",
    });
    expect(replay.mock.calls[2]).toEqual([
      "runs.steer",
      original,
      expect.objectContaining({
        idempotencyKey: pending.idempotencyKey,
        idempotencyNamespace: "idp_original",
        requestMeta: originalMeta,
      }),
    ]);
    expect(f.values.size).toBe(0);
    second.journal.dispose();
  });
  it("publishes and persists nomination before dispatch without claiming acceptance", async () => {
    const f = fixture();
    const ack = Promise.withResolvers<{ userItemId: string }>();
    let observed: string | undefined;
    const call = vi.fn((method: string) => {
      if (method === "runs.getAcceptance")
        return Promise.resolve({ type: "notRecorded", candidateUserItemId: "opaque-original" });
      expect(observed).toBe("opaque-original");
      expect([...f.values.values()][0]).toMatchObject({ steerItemId: "opaque-original" });
      return ack.promise;
    });
    const client = f.open(call as RpcClient["call"]);
    const command = client.methods.runs.steer(
      original.runId,
      original.expectedSegmentId,
      original.input,
    );
    let settled = false;
    void command.then(() => {
      settled = true;
    });
    void command.inputIdentity.then((id) => {
      observed = id;
    });
    await expect(command.inputIdentity).resolves.toBe("opaque-original");
    await vi.waitFor(() => expect(call).toHaveBeenCalledTimes(2));
    expect(settled).toBe(false);
    ack.resolve({ userItemId: "opaque-original" });
    await expect(command).resolves.toEqual({ type: "accepted", userItemId: "opaque-original" });
    client.journal.dispose();
  });

  it("refuses dispatch when the nominated identity cannot be persisted", async () => {
    const f = fixture();
    const set = f.storage.set;
    f.storage.set = (key, value) => {
      if ((value as { steerItemId: unknown }).steerItemId !== null)
        throw new Error("storage write failed");
      set(key, value);
    };
    const call = vi.fn(async (_method: string) => ({
      type: "notRecorded",
      candidateUserItemId: "item_original",
    }));
    const client = f.open(call as RpcClient["call"]);
    const command = client.methods.runs.steer(
      original.runId,
      original.expectedSegmentId,
      original.input,
    );
    await expect(command).rejects.toBeInstanceOf(MutationJournalStorageError);
    await expect(command.inputIdentity).rejects.toBeInstanceOf(MutationJournalStorageError);
    expect(call.mock.calls.map(([method]) => method)).toEqual(["runs.getAcceptance"]);
    expect([...f.values.values()][0]).toMatchObject({ steerItemId: null });
    client.journal.dispose();
  });

  it("keeps the nominated identity observable when a cold original-command lookup fails", async () => {
    const f = fixture();
    const first = f.open(
      vi.fn(async (method: string) => {
        if (method === "runs.getAcceptance")
          return { type: "notRecorded", candidateUserItemId: "item_original" };
        throw new RpcTransportError("ACK lost");
      }) as RpcClient["call"],
    );
    const submitted = first.methods.runs.steer(
      original.runId,
      original.expectedSegmentId,
      original.input,
    );
    await expect(submitted).rejects.toBeInstanceOf(RpcTransportError);
    first.journal.dispose();
    const unavailable = vi.fn(async (_method: string) => {
      throw new RpcTransportError("query unavailable");
    });
    const second = f.open(unavailable as RpcClient["call"]);
    const [recovery] = await second.methods.runs.recoverCommands();
    if (recovery?.operation !== "steer") throw new Error("expected steer");
    expect(recovery.userItemId).toBe("item_original");
    const retry = recovery.retry();
    await expect(retry.inputIdentity).resolves.toBe("item_original");
    await expect(retry).rejects.toBeInstanceOf(MutationLookupError);
    expect(unavailable.mock.calls.every(([method]) => method === "runs.getAcceptance")).toBe(true);
    expect([...f.values.values()][0]).toMatchObject({ steerItemId: "item_original" });
    second.journal.dispose();
  });

  it.each(["omitted nomination", "different ACK"])(
    "rejects %s without adopting a replacement identity",
    async (condition) => {
      const f = fixture();
      const call = vi.fn(async (method: string) =>
        method === "runs.getAcceptance"
          ? {
              type: "notRecorded",
              ...(condition === "different ACK" ? { candidateUserItemId: "item_original" } : {}),
            }
          : { userItemId: "item_other" },
      );
      const client = f.open(call as RpcClient["call"]);
      const command = client.methods.runs.steer(
        original.runId,
        original.expectedSegmentId,
        original.input,
      );
      await expect(command).rejects.toBeInstanceOf(RpcProtocolError);
      expect(call.mock.calls.filter(([method]) => method === "runs.steer")).toHaveLength(
        condition === "different ACK" ? 1 : 0,
      );
      expect([...f.values.values()][0]).toMatchObject({
        steerItemId: condition === "different ACK" ? "item_original" : null,
      });
      client.journal.dispose();
    },
  );
  it("returns retired original material without redispatching or claiming application", async () => {
    const f = fixture();
    const call = vi.fn(async (_method: string) => ({
      type: "retired",
      sessionId: "ses_1",
      runId: original.runId,
      segmentId: original.expectedSegmentId,
      userItemId: "item_original",
      input: original.input,
    }));
    const client = f.open(call as RpcClient["call"]);
    const command = client.methods.runs.steer(
      original.runId,
      original.expectedSegmentId,
      original.input,
    );
    await expect(command.inputIdentity).resolves.toBe("item_original");
    await expect(command).resolves.toEqual({ type: "retired", userItemId: "item_original" });
    expect(call.mock.calls.map(([method]) => method)).toEqual(["runs.getAcceptance"]);
    expect(f.values.size).toBe(0);
    client.journal.dispose();
  });
});

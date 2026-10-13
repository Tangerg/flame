import { describe, expect, it, vi } from "vitest";
import { createMethods } from "./methods";
import type { RpcCallOptions, RpcClient } from "./client";
import { createMutationJournal, MutationJournalOwnershipError } from "./mutationJournal";
import { MutationJournalError } from "./durableMutationJournal";
import { MutationLookupError } from "./mutation";
import { RpcError, RpcTransportError } from "./errors";
import { asRunId, asSessionId } from "./ids";
import type { RunAcceptance, RunRef } from "@flame/runtime-contract/wire";
import run from "@flame/runtime-contract/samples/runref.full.json";

const start = {
  sessionId: asSessionId("ses_1"),
  input: [{ type: "text" as const, text: "original" }],
};
const resume = {
  runId: asRunId("run_1"),
  responses: [
    { itemId: "item_question", response: { type: "answer" as const, answers: [["original"]] } },
  ],
};
const accepted: RunAcceptance = {
  type: "accepted",
  run: { ...run, activeSegmentId: "seg_later" } as RunRef,
  runId: "run_1",
  segmentId: "seg_original",
  sessionId: "ses_1",
  userItemId: "item_original",
};

function fixture() {
  const entries = new Map<string, unknown>();
  let now = 1_000;
  let namespace = "idp_original";
  const options = {
    storage: {
      get: (key: string) => structuredClone(entries.get(key)),
      set: (key: string, value: unknown) => void entries.set(key, structuredClone(value)),
      remove: (key: string) => void entries.delete(key),
      keys: () => [...entries.keys()],
    },
    scope: () => ({ namespace, retentionSeconds: 1 }),
    now: () => now,
  };
  const journal = createMutationJournal(options);
  const call =
    vi.fn<(method: string, params: unknown, options?: RpcCallOptions) => Promise<unknown>>();
  const subscribe = vi.fn(() => () => {});
  const onStreamEnd = vi.fn(() => () => {});
  const rpc = { call, subscribe, onStreamEnd } as unknown as RpcClient;
  return {
    entries,
    options,
    journal,
    call,
    subscribe,
    methods: createMethods(rpc, { mutationJournal: journal }),
    restart: () => createMethods(rpc, { mutationJournal: createMutationJournal(options) }),
    expire: () => {
      now = 2_000;
    },
    replace: () => {
      namespace = "idp_replaced";
    },
  };
}

function open(methods: ReturnType<typeof createMethods>, operation: "start" | "resume") {
  return operation === "start"
    ? methods.runs.start(structuredClone(start))
    : methods.runs.resume(structuredClone(resume));
}

describe("run command acceptance", () => {
  it.each(["start", "resume"] as const)(
    "settles a lost %s ACK after expiry without redelivery or rewriting its Segment",
    async (operation) => {
      const f = fixture();
      f.call
        .mockResolvedValueOnce({ type: "notRecorded" })
        .mockImplementationOnce(async () => {
          f.expire();
          throw new RpcTransportError("opening acknowledgement lost");
        })
        .mockResolvedValueOnce(accepted);
      const pending = open(f.methods, operation);
      const result = await pending;
      expect(result).toEqual({
        result: { runId: "run_1", segmentId: "seg_original", userItemId: "item_original" },
        observation: { type: "snapshot", sessionId: "ses_1" },
      });
      expect(f.call.mock.calls.map(([method]) => method)).toEqual([
        "runs.getAcceptance",
        `runs.${operation}`,
        "runs.getAcceptance",
      ]);
      const lookups = f.call.mock.calls.filter(([method]) => method === "runs.getAcceptance");
      for (const [, params, options] of lookups) {
        expect(params).toEqual({
          namespace: "idp_original",
          key: pending.idempotencyKey,
          operation,
        });
        expect(options).not.toHaveProperty("idempotencyKey");
        expect(options).not.toHaveProperty("idempotencyNamespace");
      }
      const writes = f.call.mock.calls.filter(([method]) => method === `runs.${operation}`);
      expect(writes).toHaveLength(1);
      expect(writes[0]?.[2]).toMatchObject({
        idempotencyKey: pending.idempotencyKey,
        idempotencyNamespace: "idp_original",
      });
      expect(writes[0]?.[1]).toEqual(operation === "start" ? start : resume);
      expect(f.subscribe).toHaveBeenCalledOnce();
      expect(f.entries.size).toBe(0);
    },
  );

  it("freezes prepared input before lookup and reuses it on authorized redelivery", async () => {
    const f = fixture();
    const lookup = Promise.withResolvers<RunAcceptance>();
    f.call
      .mockImplementationOnce(() => lookup.promise)
      .mockRejectedValueOnce(new RpcTransportError("unrecorded delivery lost"))
      .mockResolvedValueOnce({ type: "notRecorded" })
      .mockResolvedValueOnce({ runId: "run_1", segmentId: "seg_1", userItemId: "item_1" });
    const params = structuredClone(start);
    const opening = f.methods.runs.start(params);
    await vi.waitFor(() => expect(f.call).toHaveBeenCalledOnce());
    params.input[0]!.text = "changed after submission";
    lookup.resolve({ type: "notRecorded" });
    const result = await opening;
    const writes = f.call.mock.calls.filter(([method]) => method === "runs.start");
    expect(writes.map(([, input]) => input)).toEqual([start, start]);
    expect(writes.map(([, , options]) => options?.idempotencyKey)).toEqual([
      opening.idempotencyKey,
      opening.idempotencyKey,
    ]);
    expect(result.observation.type).toBe("stream");
    if (result.observation.type !== "stream") throw new Error("expected an opening stream");
    await result.observation.events[Symbol.asyncIterator]().return?.();
    expect(f.entries.size).toBe(0);
  });

  it("retains an expired notRecorded command through retries and process loss without renewing its age", async () => {
    const f = fixture();
    f.call.mockImplementation(async (method) => {
      if (method === "runs.getAcceptance") return { type: "notRecorded" };
      f.expire();
      throw new RpcTransportError("opening acknowledgement lost");
    });
    const pending = f.methods.runs.start(start);
    const original = structuredClone([...f.entries]);
    await expect(pending).rejects.toBeInstanceOf(MutationJournalError);
    await expect(pending.retry()).rejects.toBeInstanceOf(MutationJournalError);
    f.journal.dispose();
    const cold = f.restart().runs.start(start);
    expect(cold.idempotencyKey).toBe(pending.idempotencyKey);
    await expect(cold).rejects.toBeInstanceOf(MutationJournalError);
    expect([...f.entries]).toEqual(original);
    expect(f.call.mock.calls.filter(([method]) => method === "runs.start")).toHaveLength(1);
    f.call.mockResolvedValue(accepted);
    await expect(cold.retry()).resolves.toMatchObject({ observation: { type: "snapshot" } });
    expect(f.entries.size).toBe(0);
  });

  it.each(["start", "resume"] as const)(
    "retains %s identity when the read fails with a definitive-looking RPC problem",
    async (operation) => {
      const f = fixture();
      const failure = new RpcError({ message: "invalid_params", data: { type: "invalid_params" } });
      f.call.mockRejectedValueOnce(failure).mockResolvedValueOnce(accepted);
      const pending = open(f.methods, operation);
      await expect(pending).rejects.toMatchObject({ name: "MutationLookupError", cause: failure });
      expect(f.entries.size).toBe(1);
      f.expire();
      await expect(pending.retry()).resolves.toMatchObject({ observation: { type: "snapshot" } });
      expect(f.call.mock.calls.every(([method]) => method === "runs.getAcceptance")).toBe(true);
      expect(f.subscribe).not.toHaveBeenCalled();
      expect(f.entries.size).toBe(0);
    },
  );

  it("does not reinterpret a retired receipt as replacement Run material", async () => {
    const f = fixture();
    f.call.mockResolvedValue({ ...accepted, type: "retired", run: undefined });
    const pending = f.methods.runs.start(start);
    await expect(pending).resolves.toEqual({
      result: { runId: "run_1", segmentId: "seg_original", userItemId: "item_original" },
      observation: { type: "retired", sessionId: "ses_1" },
    });
    expect(f.call).toHaveBeenCalledOnce();
    expect(f.subscribe).not.toHaveBeenCalled();
    expect(f.entries.size).toBe(0);
  });

  it.each(["accepted", "notRecorded"] as const)(
    "queries the original namespace after store replacement when %s",
    async (outcome) => {
      const f = fixture();
      f.call.mockRejectedValueOnce(new RpcTransportError("lookup lost"));
      const first = f.methods.runs.start(start);
      await expect(first).rejects.toBeInstanceOf(MutationLookupError);
      f.replace();
      f.expire();
      f.journal.dispose();
      f.call.mockResolvedValue(outcome === "accepted" ? accepted : { type: "notRecorded" });
      const cold = f.restart().runs.start(start);
      expect(cold.idempotencyKey).toBe(first.idempotencyKey);
      const observed = await cold.catch((error: unknown) => error);
      const expected = {
        accepted: { observation: { type: "snapshot", sessionId: "ses_1" } },
        notRecorded: { name: "MutationJournalOwnershipError" },
      };
      expect(observed).toMatchObject(expected[outcome]);
      expect(observed instanceof MutationJournalOwnershipError).toBe(outcome === "notRecorded");
      expect(f.entries.size).toBe(outcome === "accepted" ? 0 : 1);
      expect(f.call.mock.calls.at(-1)?.[1]).toEqual({
        namespace: "idp_original",
        key: first.idempotencyKey,
        operation: "start",
      });
      expect(f.call.mock.calls.every(([method]) => method === "runs.getAcceptance")).toBe(true);
    },
  );

  it("exposes a pure typed lookup without command params, claims or mutation headers", async () => {
    const f = fixture();
    f.call.mockResolvedValue(accepted);
    await expect(
      f.methods.runs.getAcceptance({
        namespace: "idp_old",
        key: "original-key",
        operation: "resume",
      }),
    ).resolves.toEqual(accepted);
    expect(f.call).toHaveBeenCalledWith(
      "runs.getAcceptance",
      { namespace: "idp_old", key: "original-key", operation: "resume" },
      { signal: undefined },
    );
    expect(f.entries.size).toBe(0);
    expect(f.subscribe).not.toHaveBeenCalled();
  });
});

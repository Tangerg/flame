import { describe, expect, it, vi } from "vitest";
import { createMethods } from "./methods";
import type { RpcClient } from "./client";
import { RpcTransportError } from "./errors";
import { createMutationJournal, type MutationJournalStorage } from "./mutationJournal";
import { asRunId, asSegmentId } from "./ids";
import discovery from "../samples/method.discover.resp.json";
import run from "../samples/runref.full.json";

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
  const open = (call: RpcClient["call"]) => {
    const journal = createMutationJournal({
      storage,
      now: () => now,
      scope: () => ({ namespace, retentionSeconds: 60 }),
    });
    const methods = createMethods({ call } as RpcClient, {
      mutationJournal: journal,
      capabilities: () => discovery.capabilities as import("../wire.generated").ServerCapabilities,
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
          : { type: "notRecorded" };
      accepted = true;
      throw new RpcTransportError("ACK lost after admission");
    });
    const first = f.open(call as RpcClient["call"]);
    const submitted = first.methods.runs.steer(
      original.runId,
      original.expectedSegmentId,
      original.input,
    );
    await expect(submitted).resolves.toEqual({ userItemId: "item_original" });
    expect(call.mock.calls.filter(([method]) => method === "runs.steer")).toHaveLength(1);
    first.journal.dispose();

    // Model a process dying while the first response is still unavailable.
    accepted = false;
    const lost = f.open(
      vi.fn(async (method: string) => {
        if (method === "runs.getAcceptance") return { type: "notRecorded" };
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
    const [command] = await recovered.methods.runs.recoverSteers();
    expect(command!.idempotencyKey).toBe(pending.idempotencyKey);
    command!.params.input[0] = { type: "text", text: "modified public descriptor" };
    expect(command!.observation).toMatchObject({
      type: "known",
      acceptance: { userItemId: "item_original", input: original.input },
    });
    expect(call.mock.calls.map(([method]) => method)).toEqual(["runs.getAcceptance"]);
    expect(f.values.size).toBe(0);
    await expect(command!.retry()).resolves.toEqual({ userItemId: "item_original" });
    expect(call.mock.calls.every(([method]) => method === "runs.getAcceptance")).toBe(true);
    expect(stored).toHaveLength(1);
    recovered.journal.dispose();
  });

  it.each(["expired", "replaced", "lookup failure"])(
    "keeps %s evidence without granting a new delivery",
    async (condition) => {
      const f = fixture();
      const call = vi.fn(async (method: string) => {
        if (method === "runs.getAcceptance") return { type: "notRecorded" };
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
      const [command] = await second.methods.runs.recoverSteers();
      expect(command!.observation.type).toBe(condition === "lookup failure" ? "unknown" : "known");
      await expect(command!.retry()).rejects.toBeDefined();
      expect(call.mock.calls.every(([method]) => method === "runs.getAcceptance")).toBe(true);
      expect([...f.values.values()]).toEqual(saved);
      second.journal.dispose();
    },
  );

  it("retries only the persisted body and Segment under the same key after a pure not-recorded query", async () => {
    const f = fixture();
    const call = vi.fn(async (method: string) => {
      if (method === "runs.getAcceptance") return { type: "notRecorded" };
      throw new RpcTransportError("lost");
    });
    const first = f.open(call as RpcClient["call"]);
    const pending = first.methods.runs.steer(
      original.runId,
      original.expectedSegmentId,
      original.input,
    );
    await expect(pending).rejects.toBeInstanceOf(RpcTransportError);
    first.journal.dispose();
    const replay = vi.fn(async (method: string) =>
      method === "runs.getAcceptance" ? { type: "notRecorded" } : { userItemId: "item_original" },
    );
    const second = f.open(replay as RpcClient["call"]);
    const [command] = await second.methods.runs.recoverSteers();
    expect(command!.observation).toEqual({ type: "known", acceptance: { type: "notRecorded" } });
    expect(replay).toHaveBeenCalledTimes(1);
    command!.params.expectedSegmentId = asSegmentId("seg_new");
    await expect(command!.retry()).resolves.toEqual({ userItemId: "item_original" });
    expect(replay.mock.calls[2]).toEqual([
      "runs.steer",
      original,
      expect.objectContaining({
        idempotencyKey: pending.idempotencyKey,
        idempotencyNamespace: "idp_original",
      }),
    ]);
    expect(f.values.size).toBe(0);
    second.journal.dispose();
  });
});

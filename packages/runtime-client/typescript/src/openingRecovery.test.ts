import { mkdtempSync, readFileSync, writeFileSync, rmSync, readdirSync, unlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMethods, type RunCommandRecovery } from "./methods";
import type { RpcClient, RpcCallOptions } from "./client";
import { RpcTransportError } from "./errors";
import { createMutationJournal, type MutationJournalStorage } from "./mutationJournal";
import type {
  StartRunRequest,
  ResumeRunRequest,
  ServerCapabilities,
  RequestMeta,
} from "./generated/wire.generated";
import discovery from "../../../../runtime/contract/samples/method.discover.resp.json";
import run from "../../../../runtime/contract/samples/runref.full.json";

const directories: string[] = [];
afterEach(() => {
  for (const path of directories.splice(0)) rmSync(path, { recursive: true });
});

function diskStorage(directory: string): MutationJournalStorage {
  const path = (key: string) => join(directory, encodeURIComponent(key));
  return {
    get: (key) =>
      readdirSync(directory).includes(encodeURIComponent(key))
        ? JSON.parse(readFileSync(path(key), "utf8"))
        : undefined,
    set: (key, value) => writeFileSync(path(key), JSON.stringify(value)),
    remove: (key) => unlinkSync(path(key)),
    keys: () => readdirSync(directory).map(decodeURIComponent),
  };
}

const start: StartRunRequest = {
  sessionId: "ses_original",
  input: [
    { type: "text", text: "original input" },
    { type: "image", mime: "image/png", data: "aW1hZ2U=" },
  ],
  provider: "original-provider",
  model: "original-model",
  reasoningEffort: "high",
  params: { temperature: 0.3, stop: ["original-stop"] },
};
const resume: ResumeRunRequest = {
  runId: "run_original",
  input: [{ type: "text", text: "additional original input" }],
  responses: [
    {
      itemId: "item_approval",
      response: {
        type: "approval",
        decision: "approve",
        editedArgs: { path: "/original", nested: { value: 1 } },
        reason: "reviewed reason",
        remember: { scope: "project" },
      },
    },
    {
      itemId: "item_question",
      response: { type: "answer", answers: [["original answer"], ["second choice"]] },
    },
  ],
};

function fixture() {
  const directory = mkdtempSync(join(tmpdir(), "flame-original-opening-"));
  directories.push(directory);
  let now = 1000;
  let namespace = "idp_original";
  const open = (call: RpcClient["call"], requestMeta?: RequestMeta) => {
    const storage = diskStorage(directory);
    const journal = createMutationJournal({
      storage,
      now: () => now,
      scope: () => ({ namespace, retentionSeconds: 60 }),
    });
    const unsubscribe = vi.fn();
    const rpc = {
      call,
      subscribe: vi.fn(() => unsubscribe),
      onStreamEnd: vi.fn(() => unsubscribe),
    } as unknown as RpcClient;
    return {
      storage,
      journal,
      methods: createMethods(rpc, {
        mutationJournal: journal,
        capabilities: () => discovery.capabilities as ServerCapabilities,
        requestMeta: () => requestMeta,
      }),
      unsubscribe,
    };
  };
  const evidence = () =>
    readdirSync(directory).map((name) => readFileSync(join(directory, name), "utf8"));
  return {
    open,
    evidence,
    expire: () => {
      now += 61_000;
    },
    replace: () => {
      namespace = "idp_replaced";
    },
  };
}

async function leaveUncertain(f: ReturnType<typeof fixture>, operation: "start" | "resume") {
  const original = structuredClone(operation === "start" ? start : resume);
  const first = f.open(
    vi.fn(async (method: string, _params: unknown, options?: RpcCallOptions) => {
      if (method === "runs.getAcceptance") return { type: "notRecorded" };
      options?.onRequestRpcId?.("request_lost");
      throw new RpcTransportError("application died after sending the opening");
    }) as RpcClient["call"],
  );
  const submission =
    operation === "start"
      ? first.methods.runs.start(original as StartRunRequest)
      : first.methods.runs.resume(original as ResumeRunRequest);
  await expect(submission).rejects.toBeInstanceOf(RpcTransportError);
  original.input = [{ type: "text", text: "replacement draft" }];
  if ("responses" in original) original.responses = [];
  first.journal.dispose();
  return submission.idempotencyKey;
}

function assertOperation(command: RunCommandRecovery | undefined, operation: "start" | "resume") {
  if (!command || command.operation !== operation) throw new Error("original opening missing");
  return command;
}

describe.each(["start", "resume"] as const)("file-backed cold original %s", (operation) => {
  it("redelivers the original admission metadata after client preferences change", async () => {
    const f = fixture();
    const originalMeta: RequestMeta = {
      clientCapabilities: {
        interruptTypes: ["approval", "question"],
        features: { subagents: { enabled: true } },
      },
    };
    const expectedMeta = structuredClone(originalMeta);
    const first = f.open(
      vi.fn(async (method: string) => {
        if (method === "runs.getAcceptance") return { type: "notRecorded" };
        throw new RpcTransportError("lost original response");
      }) as RpcClient["call"],
      originalMeta,
    );
    const mutation =
      operation === "start" ? first.methods.runs.start(start) : first.methods.runs.resume(resume);
    await expect(mutation).rejects.toBeInstanceOf(RpcTransportError);
    originalMeta.clientCapabilities!.features!.subagents!.enabled = false;
    first.journal.dispose();
    const call = vi.fn(async (method: string, _params: unknown, options?: RpcCallOptions) => {
      if (method === "runs.getAcceptance") return { type: "notRecorded" };
      expect(options?.requestMeta).toEqual(expectedMeta);
      throw new RpcTransportError("keep original evidence");
    });
    const second = f.open(call as RpcClient["call"], originalMeta);
    const recovered = assertOperation((await second.methods.runs.recoverCommands())[0], operation);
    await expect(recovered.retry()).rejects.toBeInstanceOf(RpcTransportError);
    expect(call.mock.calls.some(([method]) => method === `runs.${operation}`)).toBe(true);
  });
  it("reads the original receipt without writing or dispatching and observes accepted/retired material after expiry", async () => {
    for (const receiptType of ["accepted", "retired"] as const) {
      const f = fixture();
      const key = await leaveUncertain(f, operation);
      const stored = f.evidence();
      const receipt = {
        type: receiptType,
        sessionId: "ses_original",
        runId: "run_original",
        segmentId: "seg_original",
        userItemId: "item_original",
        ...(receiptType === "accepted" ? { run } : {}),
      };
      const call = vi.fn(async (_method: string) => receipt);
      f.expire();
      f.replace();
      const cold = f.open(call as RpcClient["call"]);
      const command = assertOperation((await cold.methods.runs.recoverCommands())[0], operation);
      expect(command.params).toEqual(operation === "start" ? start : resume);
      expect(command.idempotencyKey).toBe(key);
      expect(call.mock.calls).toEqual([
        [
          "runs.getAcceptance",
          { operation, namespace: "idp_original", key },
          expect.objectContaining({ signal: undefined }),
        ],
      ]);
      expect(command.observation).toEqual({ type: "known", acceptance: receipt });
      expect(f.evidence()).toEqual([]);
      await expect(command.retry()).resolves.toMatchObject({
        result: { runId: "run_original", segmentId: "seg_original" },
        observation: {
          type: receiptType === "retired" ? "retired" : "snapshot",
          sessionId: "ses_original",
        },
      });
      expect(call.mock.calls.every(([method]) => method === "runs.getAcceptance")).toBe(true);
      expect(cold.unsubscribe).not.toHaveBeenCalled();
      expect(stored).toHaveLength(1);
      cold.journal.dispose();
    }
  });

  it("dispatches only the frozen original request after an explicit retry under unchanged identity and lifetime", async () => {
    const f = fixture();
    const key = await leaveUncertain(f, operation);
    const evidence = f.evidence();
    const call = vi.fn(async (method: string, _params: unknown, options?: RpcCallOptions) => {
      if (method === "runs.getAcceptance") return { type: "notRecorded" };
      options?.onRequestRpcId?.("request_retried");
      return { runId: "run_original", segmentId: "seg_original", userItemId: "item_original" };
    });
    const cold = f.open(call as RpcClient["call"]);
    const command = assertOperation((await cold.methods.runs.recoverCommands())[0], operation);
    expect(call).toHaveBeenCalledTimes(1);
    expect(f.evidence()).toEqual(evidence);
    command.params.input = [{ type: "text", text: "edited public descriptor" }];
    if (command.operation === "resume") command.params.responses = [];
    const opened = await command.retry();
    expect(call.mock.calls[2]).toEqual([
      `runs.${operation}`,
      operation === "start" ? start : resume,
      expect.objectContaining({ idempotencyKey: key, idempotencyNamespace: "idp_original" }),
    ]);
    expect(opened.observation.type).toBe("stream");
    if (opened.observation.type === "stream")
      await opened.observation.events[Symbol.asyncIterator]().return?.();
    expect(f.evidence()).toEqual([]);
    cold.journal.dispose();
  });

  it.each(["expired", "replaced", "query unavailable"])(
    "retains %s uncertainty and forbids a replacement dispatch",
    async (condition) => {
      const f = fixture();
      await leaveUncertain(f, operation);
      const evidence = f.evidence();
      if (condition === "expired") f.expire();
      if (condition === "replaced") f.replace();
      const call = vi.fn(async (_method: string) => {
        if (condition === "query unavailable")
          throw new RpcTransportError("receipt query unavailable");
        return { type: "notRecorded" };
      });
      const cold = f.open(call as RpcClient["call"]);
      const command = assertOperation((await cold.methods.runs.recoverCommands())[0], operation);
      await expect(command.retry()).rejects.toBeDefined();
      expect(call.mock.calls.every(([method]) => method === "runs.getAcceptance")).toBe(true);
      expect(f.evidence()).toEqual(evidence);
      cold.journal.dispose();
    },
  );
});

describe("original opening evidence integrity", () => {
  it.each(["former shape", "changed request", "missing metadata", "changed metadata"])(
    "preserves %s bytes and refuses any lookup or write",
    async (damage) => {
      const f = fixture();
      await leaveUncertain(f, "resume");
      const cold = f.open(vi.fn() as RpcClient["call"]);
      const key = cold.storage.keys()[0]!;
      const entry = cold.storage.get(key) as Record<string, unknown>;
      if (damage === "former shape") {
        entry.steer = null;
        delete entry.command;
      } else if (damage === "changed request")
        (entry.command as { params: ResumeRunRequest }).params.responses = [];
      else if (damage === "missing metadata")
        delete (entry.command as { requestMeta?: RequestMeta | null }).requestMeta;
      else
        (entry.command as { requestMeta: RequestMeta }).requestMeta = {
          clientInfo: { name: "changed", version: "1" },
        };
      cold.storage.set(key, entry);
      const bytes = f.evidence();
      await expect(cold.methods.runs.recoverCommands()).rejects.toThrow("corrupted");
      expect(f.evidence()).toEqual(bytes);
      cold.journal.dispose();
    },
  );

  it("allows only one active owner to retry the same recovered opening", async () => {
    const f = fixture();
    const key = await leaveUncertain(f, "start");
    const receipt = Promise.withResolvers<{ type: "notRecorded" }>();
    let held = false;
    const call = vi.fn(async (method: string, _params: unknown, options?: RpcCallOptions) => {
      if (method === "runs.getAcceptance") return held ? receipt.promise : { type: "notRecorded" };
      options?.onRequestRpcId?.("request_single");
      return { runId: "run_original", segmentId: "seg_original", userItemId: "item_original" };
    });
    const cold = f.open(call as RpcClient["call"]);
    const first = assertOperation((await cold.methods.runs.recoverCommands())[0], "start");
    const second = assertOperation((await cold.methods.runs.recoverCommands())[0], "start");
    held = true;
    const retry = first.retry();
    await expect(second.retry()).rejects.toThrow("already owned");
    expect(retry.idempotencyKey).toBe(key);
    receipt.resolve({ type: "notRecorded" });
    const opening = await retry;
    if (opening.observation.type === "stream")
      await opening.observation.events[Symbol.asyncIterator]().return?.();
    expect(call.mock.calls.filter(([method]) => method === "runs.start")).toHaveLength(1);
    expect(f.evidence()).toEqual([]);
    cold.journal.dispose();
  });
});

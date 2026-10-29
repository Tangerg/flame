import { afterEach, describe, expect, it, vi } from "vitest";
import { RpcError, RpcProtocolError, RpcTransportError } from "./errors";
import { createMutationPromise, type MutationPromise } from "./mutation";
import { parseJSON, stringifyJSON } from "@qimu/sdk/json";
import {
  createMutationJournal,
  MutationJournalCapacityError,
  MutationJournalOwnershipError,
  MutationJournalScopeUnavailableError,
  MutationJournalStorageError,
  type MutationJournal,
  type MutationJournalScope,
  type MutationJournalStorage,
  type MutationReservation,
} from "./mutationJournal";

class MemoryStorage implements MutationJournalStorage {
  readonly values = new Map<string, unknown>();

  get(key: string): unknown {
    const value = this.values.get(key);
    return value === undefined ? undefined : structuredClone(value);
  }

  set(key: string, value: unknown): void {
    this.values.set(key, structuredClone(value));
  }

  remove(key: string): void {
    this.values.delete(key);
  }

  keys(): string[] {
    return [...this.values.keys()];
  }
}

function scope(namespace = "idp_runtime_store"): MutationJournalScope {
  return {
    namespace,
    retentionSeconds: 86_400,
  };
}

function deliver<T>(
  reservation: MutationReservation,
  execute: () => Promise<T> | T,
): MutationPromise<T> {
  return reservation.track(() =>
    createMutationPromise(async () => {
      reservation.authorizeDispatch();
      return execute();
    }, reservation.idempotencyKey),
  );
}

const journals: MutationJournal[] = [];

function journal(
  storage: MutationJournalStorage,
  currentScope: () => MutationJournalScope | null | undefined = () => scope(),
): MutationJournal {
  const created = createMutationJournal({ storage, scope: currentScope });
  journals.push(created);
  return created;
}

afterEach(() => {
  for (const current of journals.splice(0).reverse()) current.dispose();
});

describe("mutation journal", () => {
  it("retains exact edited Tool arguments across persistence and original-command recovery", () => {
    const values = new Map<string, string>();
    const storage: MutationJournalStorage = {
      get: (key) => (values.has(key) ? parseJSON(values.get(key)!) : undefined),
      set: (key, value) => {
        values.set(key, stringifyJSON(value));
      },
      remove: (key) => {
        values.delete(key);
      },
      keys: () => [...values.keys()],
    };
    const params = {
      runId: "run_1",
      responses: [
        {
          itemId: "tool_1",
          response: {
            type: "approval",
            decision: "approve",
            editedArgs: parseJSON('{"wide":9007199254740993,"decimal":1.0}'),
          },
        },
      ],
    };
    const first = journal(storage);
    first.reserve("runs.resume", params, "original-exact");
    first.dispose();
    const pending = journal(storage).pendingCommands();
    expect(pending).toHaveLength(1);
    expect(pending[0]!.idempotencyKey).toBe("original-exact");
    expect(stringifyJSON(pending[0]!.params, true)).toBe(stringifyJSON(params, true));
  });
  it("keeps independent connection journals alive until their own owner retires them", async () => {
    const firstStorage = new MemoryStorage();
    const first = journal(firstStorage);
    const firstReservation = first.reserve("runs.start", {
      sessionId: "ses_1",
      input: [{ type: "text", text: "first" }],
    })!;
    const secondStorage = new MemoryStorage();
    const second = journal(secondStorage, () => scope("idp_other"));
    const secondReservation = second.reserve("runs.start", {
      sessionId: "ses_1",
      input: [{ type: "text", text: "second" }],
    })!;
    await expect(deliver(firstReservation, () => "first result")).resolves.toBe("first result");
    first.dispose();
    await expect(deliver(secondReservation, () => "second result")).resolves.toBe("second result");
    expect(firstStorage.keys()).toEqual([]);
    expect(secondStorage.keys()).toEqual([]);
  });

  it("keeps non-run mutation bodies out of the journal", async () => {
    const storage = new MemoryStorage();
    const current = journal(storage);
    const reservation = current.reserve("sessions.create", { title: "never persist me" })!;

    expect(storage.keys()).toHaveLength(1);
    const entry = storage.get(storage.keys()[0]!) as Record<string, unknown>;
    expect(Object.keys(entry).sort()).toEqual([
      "command",
      "createdAt",
      "expiresAt",
      "fingerprint",
      "idempotencyKey",
      "method",
      "namespace",
      "requestMeta",
      "salt",
      "steerItemId",
    ]);
    expect(JSON.stringify(entry)).not.toContain("never persist me");
    expect(entry).not.toHaveProperty("owner");
    expect(entry).not.toHaveProperty("generation");
    expect(entry).not.toHaveProperty("claimable");
    expect(entry).not.toHaveProperty("settled");

    await expect(deliver(reservation, () => "ok")).resolves.toBe("ok");
    expect(storage.keys()).toEqual([]);
  });

  it("uses process-local claims to separate concurrent identical commands", async () => {
    const storage = new MemoryStorage();
    const current = journal(storage);
    const first = current.reserve("sessions.fork", {
      sessionId: "ses_1",
    })!;
    const second = current.reserve("sessions.fork", {
      sessionId: "ses_1",
    })!;

    expect(second.idempotencyKey).not.toBe(first.idempotencyKey);
    expect(storage.keys()).toHaveLength(2);
    await Promise.all([deliver(first, () => undefined), deliver(second, () => undefined)]);
    expect(storage.keys()).toEqual([]);
  });

  it("settles the original identity after retirement without granting another delivery", async () => {
    const storage = new MemoryStorage();
    const predecessor = journal(storage);
    const firstReservation = predecessor.reserve("runs.start", {
      sessionId: "ses_1",
      input: [{ type: "text", text: "hello" }],
    })!;
    const firstResult = Promise.withResolvers<string>();
    const first = deliver(firstReservation, () => firstResult.promise);
    await vi.waitFor(() => expect(storage.keys()).toHaveLength(1));

    const successor = journal(storage);
    predecessor.dispose();
    const replayReservation = successor.reserve("runs.start", {
      sessionId: "ses_1",
      input: [{ type: "text", text: "hello" }],
    })!;
    expect(replayReservation.idempotencyKey).toBe(firstReservation.idempotencyKey);
    const replayResult = Promise.withResolvers<string>();
    const replay = deliver(replayReservation, () => replayResult.promise);

    firstResult.resolve("late predecessor result");
    await expect(first).resolves.toBe("late predecessor result");
    expect(storage.keys()).toEqual([]);
    await expect(first.retry()).rejects.toBeInstanceOf(MutationJournalOwnershipError);
    predecessor.dispose();
    expect(storage.keys()).toEqual([]);

    replayResult.resolve("successor result");
    await expect(replay).resolves.toBe("successor result");
    expect(storage.keys()).toEqual([]);
  });

  it("reuses an unresolved identity when the same Runtime store changes transport binding", () => {
    const storage = new MemoryStorage();
    const bind = (endpoint: string) => ({
      endpoint,
      journal: journal(storage),
    });
    const predecessor = bind("http://127.0.0.1:17171");
    const pending = predecessor.journal.reserve("runs.start", {
      sessionId: "ses_1",
      input: [{ type: "text", text: "hello" }],
    })!;

    const successor = bind("http://127.0.0.1:27272");
    const replay = successor.journal.reserve("runs.start", {
      sessionId: "ses_1",
      input: [{ type: "text", text: "hello" }],
    })!;

    expect(successor.endpoint).not.toBe(predecessor.endpoint);
    expect(replay.idempotencyKey).toBe(pending.idempotencyKey);
    expect(storage.keys()).toHaveLength(1);
  });

  it("keeps an older retry handle from borrowing another active recovery claim", async () => {
    const storage = new MemoryStorage();
    const current = journal(storage);
    const params = { sessionId: "ses_1", input: [{ type: "text", text: "original" }] };
    const original = current.reserve("runs.start", params);
    const unknown = new RpcProtocolError("acknowledgement", [{ path: "$", detail: "lost" }]);
    const execute = vi.fn().mockRejectedValue(unknown);
    const first = deliver(original, execute);
    await expect(first).rejects.toBe(unknown);
    const recovered = current.pendingCommands()[0]!.reserve();
    const result = Promise.withResolvers<string>();
    const recovery = deliver(recovered, () => result.promise);

    await expect(first.retry()).rejects.toBeInstanceOf(MutationJournalOwnershipError);
    expect(execute).toHaveBeenCalledOnce();
    expect(current.pendingCommands()).toEqual([]);
    expect(recovered.lookupNamespace()).toBe(scope().namespace);
    result.resolve("accepted");
    await expect(recovery).resolves.toBe("accepted");
    expect(storage.keys()).toEqual([]);
  });

  it.each([400, 401, 403, 404, 409, 429])(
    "preserves original command evidence when a transport replay receives HTTP %d",
    async (status) => {
      const storage = new MemoryStorage();
      const current = journal(storage);
      const params = { sessionId: "ses_1", input: [{ type: "text", text: "original" }] };
      const reservation = current.reserve("runs.start", params);
      const failure = new RpcTransportError("HTTP boundary refused the replay", status);
      const execute = vi
        .fn()
        .mockRejectedValueOnce(new RpcTransportError("original response lost"))
        .mockRejectedValueOnce(failure);
      const original = deliver(reservation, execute);
      await expect(original).rejects.toBe(failure);
      expect(execute).toHaveBeenCalledTimes(2);
      expect(current.pendingCommands()[0]!.idempotencyKey).toBe(original.idempotencyKey);
      execute.mockResolvedValueOnce("accepted");
      await expect(original.retry()).resolves.toBe("accepted");
      expect(storage.keys()).toEqual([]);
    },
  );

  it("retains an ambiguous result and reuses its exact identity on explicit retry", async () => {
    const storage = new MemoryStorage();
    const current = journal(storage);
    const reservation = current.reserve("runs.start", {
      sessionId: "ses_1",
      input: [{ type: "text", text: "hello" }],
    })!;
    const execute = vi
      .fn<() => Promise<string>>()
      .mockRejectedValueOnce(new RpcTransportError("connection reset"))
      .mockRejectedValueOnce(new RpcTransportError("connection reset"))
      .mockResolvedValueOnce("replayed");
    const first = deliver(reservation, execute);

    await expect(first).rejects.toBeInstanceOf(RpcTransportError);
    expect(storage.keys()).toHaveLength(1);
    const replay = first.retry();
    expect(replay.idempotencyKey).toBe(first.idempotencyKey);
    await expect(replay).resolves.toBe("replayed");
    expect(execute).toHaveBeenCalledTimes(3);
    expect(storage.keys()).toEqual([]);
  });

  it("keeps an unresolved identity while Runtime scope is temporarily unavailable", async () => {
    const storage = new MemoryStorage();
    let available = true;
    const current = journal(storage, () => (available ? scope() : null));
    const reservation = current.reserve("runs.start", {
      sessionId: "ses_1",
      input: [{ type: "text", text: "hello" }],
    })!;
    const execute = vi.fn(async () => {
      available = false;
      throw new RpcTransportError("discovery withdrawn");
    });
    const first = deliver(reservation, execute);

    await expect(first).rejects.toBeInstanceOf(MutationJournalScopeUnavailableError);
    expect(storage.keys()).toHaveLength(1);
    available = true;
    execute.mockResolvedValueOnce(undefined as never);
    await expect(first.retry()).resolves.toBeUndefined();
    expect(storage.keys()).toEqual([]);
  });

  it.each(["internal_error", "idempotency_conflict", "idempotency_store_mismatch"] as const)(
    "retains the exact durable restore identity after %s without automatic replay",
    async (type) => {
      const storage = new MemoryStorage();
      const current = journal(storage);
      const params = {
        sessionId: "ses_1",
        toRunId: "run_1",
        restoreType: "files",
      };
      const reservation = current.reserve("sessions.rollback", params)!;
      const original = structuredClone([...storage.values.entries()]);
      const failure = new RpcError({
        message: "restore outcome unknown",
        data: { type },
      });
      failure.cause = new RpcError({
        message: "an earlier restore was refused",
        data: { type: "checkpoint_unavailable" },
      });
      const execute = vi.fn().mockRejectedValue(failure);
      const first = deliver(reservation, execute);

      await expect(first).rejects.toBe(failure);
      expect(execute).toHaveBeenCalledOnce();
      expect([...storage.values.entries()]).toEqual(original);

      current.dispose();
      const restarted = journal(storage);
      const replay = restarted.reserve("sessions.rollback", params)!;
      expect(replay.idempotencyKey).toBe(first.idempotencyKey);
      expect([...storage.values.entries()]).toEqual(original);
      await expect(deliver(replay, () => "recovered")).resolves.toBe("recovered");
      expect(storage.keys()).toEqual([]);
    },
  );

  it.each(["checkpoint_conflict", "checkpoint_unavailable"] as const)(
    "retires a definitive %s restore refusal using its public type",
    async (type) => {
      const storage = new MemoryStorage();
      const current = journal(storage);
      const reservation = current.reserve("sessions.rollback", {
        sessionId: "ses_1",
        toRunId: "run_1",
        restoreType: "files",
      })!;
      const refusal = new RpcError({
        message: "restore did not start",
        data: { type },
      });
      refusal.cause = new RpcError({
        message: "local diagnostic",
        data: { type: "internal_error" },
      });
      const execute = vi.fn().mockRejectedValue(refusal);

      await expect(deliver(reservation, execute)).rejects.toBe(refusal);
      expect(execute).toHaveBeenCalledOnce();
      expect(storage.keys()).toEqual([]);
    },
  );

  it("retains replaced-store evidence without replaying it in the replacement", () => {
    const storage = new MemoryStorage();
    let namespace = "idp_runtime_store_a";
    const predecessor = journal(storage, () => scope(namespace));
    const old = predecessor.reserve("runs.start", {
      sessionId: "ses_1",
      input: [{ type: "text", text: "hello" }],
    })!;

    namespace = "idp_runtime_store_b";
    expect(() => old.authorizeDispatch()).toThrow(MutationJournalOwnershipError);
    const successor = journal(storage, () => scope(namespace));
    const replacement = successor.reserve("runs.start", {
      sessionId: "ses_1",
      input: [{ type: "text", text: "hello" }],
    })!;

    expect(replacement.idempotencyKey).toBe(old.idempotencyKey);
    expect(replacement.lookupNamespace()).toBe("idp_runtime_store_a");
    expect(() => replacement.authorizeDispatch()).toThrow(MutationJournalOwnershipError);
    expect(storage.keys()).toHaveLength(1);
    predecessor.dispose();
    namespace = "idp_runtime_store_a";
    const recovered = journal(storage, () => scope(namespace)).reserve("runs.start", {
      sessionId: "ses_1",
      input: [{ type: "text", text: "hello" }],
    })!;
    expect(recovered.idempotencyKey).toBe(old.idempotencyKey);
  });

  it("retains and refuses the former journal shape instead of dropping unresolved identity", () => {
    const storage = new MemoryStorage();
    const current = journal(storage);
    current.reserve("sessions.create", { title: "retained" });
    current.dispose();
    const key = storage.keys()[0]!;
    const former = storage.get(key) as Record<string, unknown>;
    delete former.method;
    storage.set(key, former);
    const successor = journal(storage);
    expect(() => successor.pendingPluginCommands()).toThrow(MutationJournalStorageError);
    expect(storage.keys()).toEqual([key]);
    expect(storage.get(key)).toEqual(former);
  });

  it("fails closed on corrupted records, identity collisions, and capacity", () => {
    const corrupted = new MemoryStorage();
    corrupted.values.set("entry:broken", { version: 1 });
    expect(() => journal(corrupted).reserve("runs.start", {})).toThrow(MutationJournalStorageError);

    const endpointShape = new MemoryStorage();
    endpointShape.values.set("entry:old-key", {
      salt: "salt",
      endpoint: "http://127.0.0.1:17171",
      namespace: "idp_runtime_store",
      fingerprint: "0".repeat(32),
      idempotencyKey: "old-key",
      createdAt: 1,
      expiresAt: 2,
    });
    expect(() => journal(endpointShape).reserve("runs.start", {})).toThrow(
      MutationJournalStorageError,
    );

    const storage = new MemoryStorage();
    const current = journal(storage);
    current.reserve(
      "runs.start",
      { sessionId: "ses_1", input: [{ type: "text", text: "first" }] },
      "fixed-key",
    );
    expect(() =>
      current.reserve(
        "runs.start",
        { sessionId: "ses_1", input: [{ type: "text", text: "second" }] },
        "fixed-key",
      ),
    ).toThrow(MutationJournalOwnershipError);
    for (let index = 1; index < 256; index++) {
      current.reserve(`runs.command.${index}`, { index });
    }
    expect(() => current.reserve("runs.command.overflow", {})).toThrow(
      MutationJournalCapacityError,
    );
  });

  it("surfaces cleanup failure without losing the retryable durable identity", async () => {
    const storage = new MemoryStorage();
    let rejectRemoval = true;
    storage.remove = (key: string) => {
      if (rejectRemoval) throw new Error("disk unavailable");
      storage.values.delete(key);
    };
    const current = journal(storage);
    const reservation = current.reserve("runs.start", {
      sessionId: "ses_1",
      input: [{ type: "text", text: "hello" }],
    })!;
    const execute = vi.fn().mockResolvedValue("done");
    const first = deliver(reservation, execute);

    await expect(first).rejects.toBeInstanceOf(MutationJournalStorageError);
    expect(storage.keys()).toHaveLength(1);
    rejectRemoval = false;
    await expect(first.retry()).resolves.toBe("done");
    expect(execute).toHaveBeenCalledTimes(2);
    expect(storage.keys()).toEqual([]);
  });

  it("does not schedule ownership heartbeats and disposal is idempotent", () => {
    const interval = vi.spyOn(globalThis, "setInterval");
    const storage = new MemoryStorage();
    const current = journal(storage);
    current.reserve("runs.start", { sessionId: "ses_1", input: [{ type: "text", text: "hello" }] });
    const persisted = structuredClone([...storage.values.entries()]);

    current.dispose();
    current.dispose();

    expect(interval).not.toHaveBeenCalled();
    expect([...storage.values.entries()]).toEqual(persisted);
    interval.mockRestore();
  });
  it("refuses the former journal shape and preserves the unresolved original bytes", () => {
    const storage = new MemoryStorage();
    const first = journal(storage);
    first.reserve("runs.steer", {
      runId: "run_1",
      expectedSegmentId: "seg_1",
      input: [{ type: "text", text: "original" }],
    });
    first.dispose();
    const key = storage.keys()[0]!;
    const old = storage.get(key) as Record<string, unknown>;
    delete old.steerItemId;
    storage.set(key, old);
    expect(() => journal(storage).pendingCommands()).toThrow(MutationJournalStorageError);
    expect(storage.get(key)).toEqual(old);
  });

  it("rejects a cached Steer identity on another command and preserves the evidence", () => {
    const storage = new MemoryStorage();
    const first = journal(storage);
    first.reserve("runs.start", {
      sessionId: "ses_1",
      input: [{ type: "text", text: "original" }],
    });
    first.dispose();
    const key = storage.keys()[0]!;
    const invalid = { ...(storage.get(key) as object), steerItemId: "item_other" };
    storage.set(key, invalid);
    expect(() => journal(storage).pendingCommands()).toThrow(MutationJournalStorageError);
    expect(storage.get(key)).toEqual(invalid);
  });
});

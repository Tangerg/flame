import { describe, expect, it, vi } from "vitest";
import { RpcError, RpcTransportError } from "./errors";
import { createMutationPromise } from "./mutation";
import {
  createPreparedMutationJournal,
  MutationJournalOwnershipError,
  MutationJournalStorageError,
  type MutationCommand,
  type MutationJournalScope,
  type MutationJournalStorage,
  type PreparedMutation,
  type PreparedMutationJournal,
} from "./mutationJournal";

class MemoryStorage implements MutationJournalStorage {
  readonly values = new Map<string, unknown>();
  get(key: string): unknown {
    return this.values.get(key);
  }
  set(key: string, value: unknown): void {
    this.values.set(key, value);
  }
  remove(key: string): void {
    this.values.delete(key);
  }
  keys(): string[] {
    return [...this.values.keys()];
  }
}

function command(): MutationCommand {
  return {
    method: "runs.start",
    params: { sessionId: "ses_1", input: [{ type: "text", text: "exact source" }] },
  };
}

function changeText(value: MutationCommand, text: string): void {
  if (value.method !== "runs.start") throw new Error("fixture must start a Run");
  const content = value.params.input[0];
  if (content?.type !== "text") throw new Error("fixture must contain text");
  content.text = text;
}

function fixture() {
  const storage = new MemoryStorage();
  const clock = { now: 1_000_000 };
  const scope: MutationJournalScope = { namespace: "idp_store", retentionSeconds: 3_600 };
  const open = () =>
    createPreparedMutationJournal({ storage, scope: () => scope, now: () => clock.now });
  return { storage, scope, clock, open, journal: open() };
}

function deliver<T>(
  journal: PreparedMutationJournal,
  prepared: PreparedMutation,
  execute: () => T | Promise<T>,
) {
  const reservation = journal.recover(prepared.idempotencyKey, prepared.method, prepared.params);
  return reservation.track(
    createMutationPromise(async () => {
      reservation.authorizeAttempt();
      return execute();
    }, reservation.idempotencyKey),
  );
}

describe("prepared mutation journal", () => {
  it.each([
    { method: "sessions.create", params: { workspace: { path: "/runtime" } } },
    command(),
    {
      method: "runs.resume",
      params: {
        runId: "run_1",
        responses: [{ itemId: "item_1", response: { type: "approval", decision: "approve" } }],
      },
    },
    {
      method: "runs.cancel",
      params: { runId: "run_1", reason: "operator requested cancellation" },
    },
  ] satisfies MutationCommand[])(
    "prepares and restores the generated $method request contract",
    (input) => {
      const { journal, open } = fixture();
      const prepared = journal.prepare(input);
      journal.dispose();
      const recovered = open();
      expect(recovered.read(prepared.idempotencyKey)).toEqual(prepared);
      expect(
        recovered.recover(prepared.idempotencyKey, input.method, input.params).authorizeAttempt(),
      ).toBe("idp_store");
    },
  );

  it("freezes exact input, isolates projections and distinguishes identical new commands", () => {
    const { journal, open } = fixture();
    const input = command();
    const prepared = journal.prepare(input);
    const original = structuredClone(prepared);
    changeText(input, "caller edited the original input");
    changeText(prepared, "caller edited the returned preparation");
    const read = journal.read(prepared.idempotencyKey)!;
    changeText(read, "caller edited the returned read");
    expect(journal.read(prepared.idempotencyKey)).toEqual(original);
    const second = journal.prepare(command());
    expect(second.idempotencyKey).not.toBe(prepared.idempotencyKey);
    journal.dispose();
    expect(() => journal.list()).toThrow(MutationJournalOwnershipError);
    expect(open().list()).toEqual([original, second]);
  });

  it("uses original age when the same Runtime shortens retention after reconnect", () => {
    const { journal, open, scope, clock, storage } = fixture();
    const prepared = journal.prepare(command());
    const original = structuredClone([...storage.values]);
    journal.dispose();
    clock.now += 61_000;
    scope.retentionSeconds = 60;
    const recovered = open();
    expect(() =>
      recovered.recover(prepared.idempotencyKey, prepared.method, prepared.params),
    ).toThrow("expired");
    expect(recovered.list()).toEqual([prepared]);
    expect([...storage.values]).toEqual(original);
  });

  it("cannot extend original expiry when the Runtime lengthens retention", () => {
    const { journal, open, scope, clock } = fixture();
    scope.retentionSeconds = 60;
    const prepared = journal.prepare(command());
    journal.dispose();
    clock.now += 61_000;
    scope.retentionSeconds = 3_600;
    const recovered = open();
    expect(() =>
      recovered.recover(prepared.idempotencyKey, prepared.method, prepared.params),
    ).toThrow("expired");
    expect(recovered.list()).toEqual([prepared]);
  });

  it("retains original-store commands and never mints a missing identity", () => {
    const { journal, scope, storage } = fixture();
    const prepared = journal.prepare(command());
    scope.namespace = "idp_other";
    expect(() =>
      journal.recover(prepared.idempotencyKey, prepared.method, prepared.params),
    ).toThrow("replaced Runtime store");
    expect(journal.list()).toEqual([prepared]);
    storage.values.clear();
    expect(journal.read(prepared.idempotencyKey)).toBeUndefined();
    expect(() =>
      journal.recover(prepared.idempotencyKey, prepared.method, prepared.params),
    ).toThrow("no longer retained");
    expect(storage.keys()).toEqual([]);
  });

  it.each(["namespace", "retention"] as const)(
    "preserves unknown acceptance when %s changes before automatic replay",
    async (change) => {
      const { journal, scope, clock, storage } = fixture();
      const prepared = journal.prepare(command());
      const original = structuredClone([...storage.values]);
      const execute = vi.fn(() => {
        if (change === "namespace") scope.namespace = "idp_replacement";
        else {
          scope.retentionSeconds = 1;
          clock.now += 2_000;
        }
        throw new RpcTransportError("acknowledgement lost");
      });
      await expect(deliver(journal, prepared, execute)).rejects.toThrow(
        change === "namespace" ? "replaced Runtime store" : "expired",
      );
      expect(execute).toHaveBeenCalledOnce();
      expect(journal.read(prepared.idempotencyKey)).toEqual(prepared);
      expect([...storage.values]).toEqual(original);
    },
  );

  it("retains unknown results across restart and settles only the recovered command", async () => {
    const { journal, open } = fixture();
    const prepared = journal.prepare(command());
    const other = journal.prepare(command());
    const execute = vi.fn(() => {
      throw new RpcTransportError("acknowledgement lost");
    });
    await expect(deliver(journal, prepared, execute)).rejects.toThrow("acknowledgement lost");
    journal.dispose();
    const recovered = open();
    await expect(deliver(recovered, prepared, () => "accepted")).resolves.toBe("accepted");
    expect(recovered.list()).toEqual([other]);
    expect(() =>
      recovered.recover(prepared.idempotencyKey, prepared.method, prepared.params),
    ).toThrow("no longer retained");
  });

  it("settles authoritative refusals through the same journal owner", async () => {
    const { journal } = fixture();
    const prepared = journal.prepare(command());
    const refusal = new RpcError({ message: "invalid_params", data: { type: "invalid_params" } });
    await expect(
      deliver(journal, prepared, () => {
        throw refusal;
      }),
    ).rejects.toBe(refusal);
    expect(journal.list()).toEqual([]);
  });

  it("releases the finished claim and preserves exact input when durable settlement fails", async () => {
    const { journal, storage } = fixture();
    const prepared = journal.prepare(command());
    const remove = vi.spyOn(storage, "remove").mockImplementationOnce(() => {
      throw new Error("disk unavailable");
    });
    await expect(deliver(journal, prepared, () => "accepted")).rejects.toBeInstanceOf(
      MutationJournalStorageError,
    );
    expect(journal.read(prepared.idempotencyKey)).toEqual(prepared);
    await expect(deliver(journal, prepared, () => "replayed acceptance")).resolves.toBe(
      "replayed acceptance",
    );
    expect(remove).toHaveBeenCalledTimes(2);
    expect(journal.list()).toEqual([]);
  });

  it.each(["changed input", "missing identity", "foreign shape"] as const)(
    "refuses %s before dispatch",
    (damage) => {
      const { journal, storage } = fixture();
      const prepared = journal.prepare(command());
      const key = storage.keys()[0]!;
      const record = storage.get(key) as Record<string, unknown>;
      if (damage === "changed input")
        record.params = {
          sessionId: "ses_other",
          input: [{ type: "text", text: "changed source" }],
        };
      else if (damage === "missing identity") record.identity = undefined;
      else
        storage.set(key, {
          id: prepared.idempotencyKey,
          namespace: "idp_store",
          expiresAt: 9_000_000,
          command: command(),
        });
      const failure =
        damage === "changed input" ? MutationJournalOwnershipError : MutationJournalStorageError;
      expect(() => journal.read(prepared.idempotencyKey)).toThrow(failure);
      expect(() =>
        journal.recover(prepared.idempotencyKey, prepared.method, prepared.params),
      ).toThrow(failure);
      expect(storage.keys()).toEqual([key]);
    },
  );

  it("rejects malformed input through the generated request contract before persistence", () => {
    const { journal, storage } = fixture();
    expect(() =>
      journal.prepare({
        method: "runs.start",
        params: { input: [] },
      } as unknown as MutationCommand),
    ).toThrow(MutationJournalStorageError);
    expect(storage.keys()).toEqual([]);
  });

  it("does not classify a record removed between storage reads as corruption", () => {
    const { journal, storage } = fixture();
    const prepared = journal.prepare(command());
    vi.spyOn(storage, "get").mockImplementationOnce((key) => {
      const value = storage.values.get(key);
      storage.values.delete(key);
      return value;
    });
    expect(journal.read(prepared.idempotencyKey)).toBeUndefined();
    journal.prepare(command());
    vi.spyOn(storage, "keys").mockImplementationOnce(() => {
      const keys = [...storage.values.keys()];
      storage.values.clear();
      return keys;
    });
    expect(journal.list()).toEqual([]);
  });
});

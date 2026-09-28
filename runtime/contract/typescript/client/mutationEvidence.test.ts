import { afterEach, expect, it, vi } from "vitest";
import { RpcError } from "./errors";
import { createMutationPromise, mutationSettlementIsUnknown } from "./mutation";
import { createMutationJournal, type MutationJournalStorage } from "./mutationJournal";
import { MutationJournalError, openDurableMutationJournal } from "./durableMutationJournal";

function storage(): MutationJournalStorage {
  const values = new Map<string, unknown>();
  return {
    get: (key) => structuredClone(values.get(key)),
    set: (key, value) => void values.set(key, structuredClone(value)),
    remove: (key) => void values.delete(key),
    keys: () => [...values.keys()],
  };
}

afterEach(() => vi.useRealTimers());

it.each(["AbortError", "TimeoutError"])("retains an in-progress command after %s", async (name) => {
  vi.useFakeTimers();
  const durable = storage();
  const options = {
    storage: durable,
    scope: () => ({ namespace: "runtime-store", retentionSeconds: 60 }),
  };
  const journal = createMutationJournal(options);
  const params = { sessionId: "ses_1" };
  const reservation = journal.reserve("runs.start", params)!;
  const controller = new AbortController();
  const cause = new DOMException("observation stopped", name);
  const mutation = reservation.track(
    createMutationPromise(
      async () => {
        reservation.authorizeAttempt();
        throw new RpcError({
          message: "still running",
          data: { type: "idempotency_in_progress", retryAfterSeconds: 2 },
        });
      },
      reservation.idempotencyKey,
      { signal: controller.signal },
    ),
  );
  const failure = mutation.catch((error: unknown) => error);
  await vi.advanceTimersByTimeAsync(0);
  controller.abort(cause);
  const error = await failure;
  expect(mutationSettlementIsUnknown(error)).toBe(true);
  expect(error).toHaveProperty("cause", cause);
  expect(durable.keys()).toHaveLength(1);
  journal.dispose();
  const recovered = createMutationJournal(options);
  try {
    expect(recovered.reserve("runs.start", params)?.idempotencyKey).toBe(
      reservation.idempotencyKey,
    );
  } finally {
    recovered.dispose();
  }
});

it("refuses expired replay without deleting evidence or issuing a replacement key", () => {
  const durable = storage();
  let now = 1_000;
  const options = {
    storage: durable,
    scope: () => ({ namespace: "runtime-store", retentionSeconds: 1 }),
    now: () => now,
  };
  const original = openDurableMutationJournal(options).reserve(
    "runs.start",
    { text: "hello" },
    undefined,
    () => false,
  )!;
  const before = durable.keys().map((key) => [key, durable.get(key)]);
  now = 2_000;
  const restarted = openDurableMutationJournal(options);
  expect(() => restarted.reserve("runs.start", { text: "hello" }, undefined, () => false)).toThrow(
    MutationJournalError,
  );
  expect(() =>
    restarted.recover("runs.start", { text: "hello" }, original.entry.idempotencyKey),
  ).toThrow(MutationJournalError);
  expect(durable.keys().map((key) => [key, durable.get(key)])).toEqual(before);
  expect(restarted.entries()[0]?.entry.idempotencyKey).toBe(original.entry.idempotencyKey);
});

import { afterEach, describe, expect, it, vi } from "vitest";
import { rejected } from "@/test/rejected";
import { RpcTransportError } from "./errors";
import type { MutationPromise } from "./mutation";
import { createMutationSettler, MutationSettlementClosedError } from "./mutationSettlement";

afterEach(() => vi.useRealTimers());

function resolvedMutation<T>(value: T): MutationPromise<T> {
  return Object.assign(Promise.resolve(value), {
    idempotencyKey: "same-key",
    retry: vi.fn(),
  });
}

describe("unary mutation settlement", () => {
  it("replays a timed-out attempt with the same logical mutation", async () => {
    vi.useFakeTimers();
    const retry = vi.fn((_options?: { signal?: AbortSignal }) => resolvedMutation("committed"));
    const open = vi.fn((signal: AbortSignal) =>
      Object.assign(
        new Promise<string>((_resolve, reject) => {
          signal.addEventListener("abort", () => reject(signal.reason), { once: true });
        }),
        { idempotencyKey: "same-key", retry },
      ),
    );

    const result = createMutationSettler().settle("test:timeout-replay", open, { timeoutMs: 10 });
    await vi.advanceTimersByTimeAsync(10);

    await expect(result).resolves.toBe("committed");
    expect(open).toHaveBeenCalledOnce();
    expect(retry).toHaveBeenCalledOnce();
    expect(retry.mock.calls[0]?.[0]?.signal).toBeInstanceOf(AbortSignal);
  });

  // The journal refuses a replay it no longer owns by throwing from `retry` itself, before any
  // promise exists. The replacement attempt is already open at that point — its deadline timer
  // is armed and it is listening to the settler's lifetime.
  it("closes the replacement attempt when the replay is refused outright", async () => {
    vi.useFakeTimers();
    const refused = new Error("journal no longer owns this key");
    const retry = vi.fn(() => {
      throw refused;
    });
    const open = vi.fn((signal: AbortSignal) =>
      Object.assign(
        new Promise<string>((_resolve, reject) => {
          signal.addEventListener("abort", () => reject(signal.reason), { once: true });
        }),
        { idempotencyKey: "same-key", retry },
      ),
    );

    const settler = createMutationSettler();
    // Held before the clock moves: the rejection lands during the advance, and a promise that
    // rejects with no handler attached is an unhandled rejection in this runner.
    const settled = rejected(settler.settle("test:refused-replay", open, { timeoutMs: 10 }));
    await vi.advanceTimersByTimeAsync(10);

    expect(await settled).toBe(refused);
    expect(vi.getTimerCount()).toBe(0);
  });

  it("settles after the finite retry budget even when the transport ignores abort", async () => {
    vi.useFakeTimers();
    let settleIgnored!: (value: string) => void;
    const ignored = new Promise<string>((resolve) => {
      settleIgnored = resolve;
    });
    let mutation!: MutationPromise<string>;
    const retry = vi.fn(() => mutation);
    mutation = Object.assign(ignored, { idempotencyKey: "same-key", retry });
    const open = vi.fn(() => mutation);

    const result = createMutationSettler().settle("test:ignored-abort", open, { timeoutMs: 10 });
    const settlement = result.then(
      () => null,
      (error: unknown) => error,
    );
    await vi.advanceTimersByTimeAsync(20);

    await expect(settlement).resolves.toMatchObject({ name: "TimeoutError" });
    expect(open).toHaveBeenCalledOnce();
    expect(retry).toHaveBeenCalledOnce();
    settleIgnored("late ignored response");
    await ignored;
  });

  it("reuses the retained mutation after an unknown timeout settlement", async () => {
    vi.useFakeTimers();
    let settleIgnored!: (value: string) => void;
    const ignored = new Promise<string>((resolve) => {
      settleIgnored = resolve;
    });
    const retry = vi.fn(() =>
      retry.mock.calls.length === 2
        ? resolvedMutation("committed")
        : Object.assign(ignored, { idempotencyKey: "same-key", retry }),
    );
    const open = vi.fn(
      () =>
        Object.assign(ignored, { idempotencyKey: "same-key", retry }) as MutationPromise<string>,
    );
    const settler = createMutationSettler();

    const first = settler.settle("goals.stop:ses_1", open, { timeoutMs: 10 });
    const firstFailure = first.catch((error: unknown) => error);
    await vi.advanceTimersByTimeAsync(20);
    await expect(firstFailure).resolves.toMatchObject({ name: "TimeoutError" });

    await expect(settler.settle("goals.stop:ses_1", open, { timeoutMs: 10 })).resolves.toBe(
      "committed",
    );
    expect(open).toHaveBeenCalledOnce();
    expect(retry).toHaveBeenCalledTimes(2);
    settleIgnored("late ignored response");
    await ignored;
  });

  it("retains protocol-ambiguous transport failures without retaining definitive failures", async () => {
    const transportFailure = new RpcTransportError("response body lost");
    const retry = vi.fn(() => resolvedMutation("replayed"));
    const ambiguousOpen = vi.fn(
      () =>
        Object.assign(Promise.reject(transportFailure), {
          idempotencyKey: "same-key",
          retry,
        }) as MutationPromise<string>,
    );
    const settler = createMutationSettler();

    await expect(settler.settle("sessions.create:/repo", ambiguousOpen)).rejects.toBe(
      transportFailure,
    );
    await expect(settler.settle("sessions.create:/repo", ambiguousOpen)).resolves.toBe("replayed");
    expect(ambiguousOpen).toHaveBeenCalledOnce();

    const refusal = new Error("definitive refusal");
    const definitiveOpen = vi
      .fn()
      .mockReturnValueOnce(
        Object.assign(Promise.reject(refusal), {
          idempotencyKey: "first-key",
          retry: vi.fn(),
        }),
      )
      .mockReturnValueOnce(resolvedMutation("new-command"));

    await expect(settler.settle("goals.resume:ses_1", definitiveOpen)).rejects.toBe(refusal);
    await expect(settler.settle("goals.resume:ses_1", definitiveOpen)).resolves.toBe("new-command");
    expect(definitiveOpen).toHaveBeenCalledTimes(2);
  });

  it("does not merge fresh same-shaped calls that may be distinct product intents", async () => {
    const open = vi.fn(() => resolvedMutation("committed"));
    const settler = createMutationSettler();

    const first = settler.settle("goals.start:ses_1", open);
    const second = settler.settle("goals.start:ses_1", open);

    await expect(Promise.all([first, second])).resolves.toEqual(["committed", "committed"]);
    expect(open).toHaveBeenCalledTimes(2);
  });

  it("revokes an in-flight attempt when its adapter generation is disposed", async () => {
    let settleRetired!: (value: string) => void;
    const retired = new Promise<string>((resolve) => {
      settleRetired = resolve;
    });
    let attemptSignal: AbortSignal | undefined;
    let mutation!: MutationPromise<string>;
    mutation = Object.assign(retired, {
      idempotencyKey: "retired-key",
      retry: vi.fn(() => mutation),
    });
    const settler = createMutationSettler();
    const settlement = settler.settle(
      "sessions.create:/repo",
      (signal) => {
        attemptSignal = signal;
        return mutation;
      },
      { timeoutMs: 60_000 },
    );

    settler.dispose();

    await expect(settlement).rejects.toBeInstanceOf(MutationSettlementClosedError);
    expect(attemptSignal?.aborted).toBe(true);
    await expect(
      settler.settle("sessions.create:/repo", () => resolvedMutation("successor")),
    ).rejects.toBeInstanceOf(MutationSettlementClosedError);

    settleRetired("late retired response");
    await retired;
  });
});

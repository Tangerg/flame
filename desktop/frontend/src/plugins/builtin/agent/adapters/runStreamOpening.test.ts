import { describe, expect, it, vi } from "vitest";
import { settleRunStreamOpening } from "./runStreamOpening";

function runStream(close: () => void) {
  return {
    result: { runId: "run_1", segmentId: "seg_1" },
    events: {
      [Symbol.asyncIterator]: () => ({
        next: () => new Promise<never>(() => {}),
        return: () => {
          close();
          return Promise.resolve({ done: true as const, value: undefined });
        },
      }),
    },
  } as unknown as Awaited<ReturnType<typeof settleRunStreamOpening>> & object;
}

describe("settleRunStreamOpening", () => {
  it("hands over a stream that arrives before the abort", async () => {
    const close = vi.fn();
    const controller = new AbortController();
    const stream = runStream(close);

    await expect(settleRunStreamOpening(Promise.resolve(stream), controller.signal)).resolves.toBe(
      stream,
    );
    expect(close).not.toHaveBeenCalled();
  });

  // The transport is not required to honour the signal, so the opening can still succeed after
  // the generation released ownership. Dropping that stream leaks a live subscription.
  it("retires a stream that arrives after the abort", async () => {
    const close = vi.fn();
    const controller = new AbortController();
    const opening = Promise.withResolvers<ReturnType<typeof runStream>>();
    const settled = settleRunStreamOpening(opening.promise, controller.signal);

    controller.abort();
    await expect(settled).resolves.toBeNull();

    opening.resolve(runStream(close));
    await vi.waitFor(() => expect(close).toHaveBeenCalledOnce());
  });

  it("does not reject after the abort has already released ownership", async () => {
    const controller = new AbortController();
    const opening = Promise.withResolvers<ReturnType<typeof runStream>>();
    const settled = settleRunStreamOpening(opening.promise, controller.signal);

    controller.abort();
    await expect(settled).resolves.toBeNull();

    opening.reject(new Error("opening failed after abort"));
    await expect(settled).resolves.toBeNull();
  });
});

import { describe, expect, it, vi } from "vitest";
import {
  ASYNC_OWNERSHIP_RETIRED,
  disposeAsyncIterator,
  settleBeforeAbort,
  settleWithinNextTask,
} from "./asyncOwnership";

describe("async ownership", () => {
  it("retires a late value after abort without releasing its observation", async () => {
    let resolve!: (value: string) => void;
    const operation = new Promise<string>((settle) => {
      resolve = settle;
    });
    const controller = new AbortController();
    const dispose = vi.fn();
    const settled = settleBeforeAbort(operation, controller.signal, dispose);

    controller.abort();
    await expect(settled).resolves.toBe(ASYNC_OWNERSHIP_RETIRED);
    resolve("late resource");
    await Promise.resolve();
    expect(dispose).toHaveBeenCalledWith("late resource");
  });

  it("distinguishes fulfillment, rejection, and a promise pending past the next task", async () => {
    await expect(settleWithinNextTask(Promise.resolve("done"))).resolves.toEqual({
      status: "fulfilled",
      value: "done",
    });
    await expect(settleWithinNextTask(Promise.reject(new Error("no")))).resolves.toEqual({
      status: "rejected",
    });
    await expect(settleWithinNextTask(new Promise<never>(() => undefined))).resolves.toEqual({
      status: "pending",
    });
  });

  it("does not let a non-cooperative iterator return hold retirement", async () => {
    const iterator: AsyncIterator<never> = {
      next: () => new Promise<IteratorResult<never>>(() => undefined),
      return: () => new Promise<IteratorResult<never>>(() => undefined),
    };
    await expect(disposeAsyncIterator(iterator)).resolves.toBeUndefined();
  });
});

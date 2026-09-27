import { describe, expect, it, vi } from "vitest";
import { RetirableTaskCohort } from "./taskQueue";

function cohort() {
  return new RetirableTaskCohort(new Error("generation retired"));
}

describe("serial work within a task cohort", () => {
  it("holds work for the same identity while other identities proceed", async () => {
    const tasks = cohort();
    const first = Promise.withResolvers<void>();
    const order: string[] = [];
    const a = tasks.runSerial("same", async () => {
      await first.promise;
      order.push("first");
    });
    const b = tasks.runSerial("same", async () => {
      order.push("second");
    });

    await tasks.runSerial("other", async () => {
      order.push("other");
    });
    expect(order).toEqual(["other"]);

    first.resolve();
    await Promise.all([a, b]);
    expect(order).toEqual(["other", "first", "second"]);
  });

  it("keeps caller failures separate while subsequent work stays queued", async () => {
    const tasks = cohort();
    const first = Promise.withResolvers<void>();
    const failed = tasks.runSerial("same", () => first.promise).catch((error: unknown) => error);
    const queued = tasks.runSerial("same", async () => "accepted");

    first.reject(new Error("save failed"));

    await expect(failed).resolves.toMatchObject({ message: "save failed" });
    await expect(queued).resolves.toBe("accepted");
  });

  it("keeps an identity occupied until its final queued operation settles", async () => {
    const tasks = cohort();
    const first = Promise.withResolvers<void>();
    const second = Promise.withResolvers<void>();
    const last = vi.fn(async () => "last");
    const a = tasks.runSerial("same", () => first.promise);
    const b = tasks.runSerial("same", () => second.promise);

    first.resolve();
    await a;
    const c = tasks.runSerial("same", last);
    await tasks.runSerial("other", async () => undefined);
    expect(last).not.toHaveBeenCalled();

    second.resolve();
    await b;
    await expect(c).resolves.toBe("last");
  });

  it("retires active and queued callers without starting queued dependencies", async () => {
    const tasks = cohort();
    const nonCooperative = Promise.withResolvers<void>();
    const first = vi.fn(() => nonCooperative.promise);
    const next = vi.fn(async () => "stale");
    const active = tasks.runSerial("same", first).catch((error: unknown) => error);
    const queued = tasks.runSerial("same", next).catch((error: unknown) => error);
    await tasks.runSerial("other", async () => undefined);
    expect(first).toHaveBeenCalledOnce();

    tasks.retire();

    await expect(active).resolves.toMatchObject({ message: "generation retired" });
    await expect(queued).resolves.toMatchObject({ message: "generation retired" });
    await expect(tasks.runSerial("same", next)).rejects.toThrow("generation retired");
    nonCooperative.resolve();
    await Promise.resolve();
    expect(next).not.toHaveBeenCalled();
  });
});

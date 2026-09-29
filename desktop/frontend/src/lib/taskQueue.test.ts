import { describe, expect, it, vi } from "vitest";
import { RetirableTaskCohort } from "./taskQueue";

describe("retirable task cohort", () => {
  it("observes an existing operation when its waiter is already retired", async () => {
    const retired = new Error("generation retired");
    const cohort = new RetirableTaskCohort(retired);
    const late = Promise.withResolvers<string>();
    const observe = vi.spyOn(late.promise, "then");
    cohort.retire();

    await expect(cohort.settle(late.promise)).rejects.toBe(retired);
    expect(observe).toHaveBeenCalledOnce();
    late.reject(new Error("late external failure"));
    await Promise.resolve();
  });

  it("observes a rejection after synchronous retirement inside the operation", async () => {
    const retired = new Error("generation retired");
    const cohort = new RetirableTaskCohort(retired);
    const late = Promise.withResolvers<string>();
    const observe = vi.spyOn(late.promise, "then");

    const command = cohort.run(() => {
      cohort.retire();
      return late.promise;
    });

    await expect(command).rejects.toBe(retired);
    expect(observe).toHaveBeenCalledOnce();
    late.reject(new Error("late external failure"));
    await Promise.resolve();
  });

  it("assimilates thenables through the native promise boundary", async () => {
    const cohort = new RetirableTaskCohort(new Error("generation retired"));
    const external = new Error("external then failed");
    let observed = false;
    const operation: PromiseLike<string> = {
      // oxlint-disable-next-line no-thenable -- this fixture exercises an external thenable that throws during assimilation
      then() {
        observed = true;
        throw external;
      },
    };

    const settlement = cohort.settle(operation);
    expect(observed).toBe(false);
    await expect(settlement).rejects.toBe(external);
    expect(observed).toBe(true);
    cohort.retire();
  });

  it("retires only pending cohort settlements and ignores non-cooperative late results", async () => {
    const retired = new Error("generation retired");
    const cohort = new RetirableTaskCohort(retired);
    await expect(cohort.settle(Promise.resolve("completed"))).resolves.toBe("completed");

    const late = Promise.withResolvers<string>();
    const settlement = cohort.settle(late.promise);
    cohort.retire();
    await expect(settlement).rejects.toBe(retired);

    late.resolve("stale");
    await Promise.resolve();
    expect(() => cohort.assertCurrent()).toThrow(retired);
  });

  it("never reaches the dependency once retired", async () => {
    const retired = new Error("generation retired");
    const cohort = new RetirableTaskCohort(retired);
    const operation = vi.fn(() => Promise.resolve("value"));
    cohort.retire();

    await expect(cohort.run(operation)).rejects.toBe(retired);
    expect(operation).not.toHaveBeenCalled();
  });

  it("refuses a value that arrived after retirement", async () => {
    const retired = new Error("generation retired");
    const cohort = new RetirableTaskCohort(retired);
    const settled = Promise.withResolvers<string>();

    const command = cohort.run(() => settled.promise);
    cohort.retire();
    settled.resolve("stale");

    await expect(command).rejects.toBe(retired);
  });
});

import { describe, expect, it, vi } from "vitest";
import { RetirableTaskCohort } from "./taskQueue";

describe("retirable task cohort", () => {
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

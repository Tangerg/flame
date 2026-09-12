import { describe, expect, it } from "vitest";
import { planSteps, SessionPlan } from "./sessionPlan";

describe("planSteps", () => {
  it("reads the Adapter-owned checklist projection", () => {
    expect(
      planSteps({
        revision: 1,
        steps: [
          { id: "1", text: "Read the code", status: "done" },
          { id: "2", text: "Write the fix", status: "active" },
          { id: "3", text: "Run tests", status: "pending" },
        ],
      }),
    ).toEqual([
      { id: "1", text: "Read the code", status: "done" },
      { id: "2", text: "Write the fix", status: "active" },
      { id: "3", text: "Run tests", status: "pending" },
    ]);
  });

  it("has no steps without a snapshot", () => {
    expect(planSteps(undefined)).toEqual([]);
  });
});

describe("SessionPlan", () => {
  const snapshot = {
    revision: 7,
    steps: [{ id: "1", text: "Inspect", status: "active" as const }],
  };

  it("uses exact projection generation, Session and whole-replacement revision as identity", () => {
    const current = SessionPlan.fromSnapshot("ses-a", 2n, snapshot);
    expect(SessionPlan.fromSnapshot("ses-a", 2n, snapshot).identity).toBe(current.identity);
    expect(SessionPlan.fromSnapshot("ses-a", 2n, { ...snapshot, revision: 8 }).identity).not.toBe(
      current.identity,
    );
    expect(SessionPlan.fromSnapshot("ses-b", 2n, snapshot).identity).not.toBe(current.identity);
    expect(SessionPlan.fromSnapshot("ses-a", 3n, snapshot).identity).not.toBe(current.identity);
  });

  it("keeps an unwritten Plan distinct from an explicitly cleared replacement", () => {
    const unwritten = SessionPlan.fromSnapshot("ses-a", 2n, undefined);
    const cleared = SessionPlan.fromSnapshot("ses-a", 2n, { revision: 1, steps: [] });

    expect(unwritten.revision).toBeUndefined();
    expect(cleared.revision).toBe(1);
    expect(unwritten.steps).toEqual([]);
    expect(cleared.steps).toEqual([]);
    expect(unwritten.identity).not.toBe(cleared.identity);
  });

  it("owns active-step and completion behavior", () => {
    const plan = SessionPlan.fromSnapshot("ses-a", 2n, {
      revision: 8,
      steps: [
        { id: "1", text: "Inspect", status: "done" },
        { id: "2", text: "Fix", status: "active" },
      ],
    });
    expect(plan.activeStep()?.text).toBe("Fix");
    expect(plan.progress()).toEqual({ done: 1, total: 2 });
  });
});

describe("SessionPlan active step", () => {
  it.each([
    ["active work after an untouched step", ["pending", "active"], "1"],
    ["the next untouched step", ["done", "pending"], "1"],
    ["a completed plan", ["done", "done"], undefined],
    ["an empty plan", [], undefined],
  ] as const)("projects %s", (_, statuses, expected) => {
    const plan = SessionPlan.fromSnapshot("ses-a", 1n, {
      revision: 1,
      steps: statuses.map((status, index) => ({ id: String(index), text: "Work", status })),
    });
    expect(plan.activeStep()?.id).toBe(expected);
  });
});

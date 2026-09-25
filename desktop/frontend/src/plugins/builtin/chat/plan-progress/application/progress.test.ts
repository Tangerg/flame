import { SessionPlan, type PlanStep } from "@/plugins/builtin/agent/public/plan";
import { describe, expect, it } from "vitest";
import { activePlanState } from "./progress";

const step = (id: number, text: string, status: PlanStep["status"]): PlanStep => ({
  id: `step-${id}`,
  text,
  status,
});

describe("activePlanState", () => {
  const plan = [step(1, "done", "done"), step(2, "current", "active"), step(3, "next", "pending")];
  const material = SessionPlan.fromSnapshot("ses-1", 1n, { revision: 3, steps: plan });

  it("reports the plan while a step is still in flight", () => {
    expect(activePlanState(material)).toMatchObject({
      visible: true,
      total: 3,
      done: 1,
      percent: 33,
      current: plan[1],
    });
  });

  it("keeps a finished plan reviewable", () => {
    const done = SessionPlan.fromSnapshot("ses-1", 1n, {
      revision: 4,
      steps: [step(1, "done", "done")],
    });
    expect(activePlanState(done)).toMatchObject({
      visible: true,
      total: 1,
      done: 1,
      percent: 100,
      current: undefined,
    });
  });

  it("stays down when the session has no plan", () => {
    const empty = SessionPlan.fromSnapshot("ses-1", 1n, { revision: 1, steps: [] });
    expect(activePlanState(empty).visible).toBe(false);
  });
});

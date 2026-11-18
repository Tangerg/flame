import { describe, expect, it } from "vitest";
import { goalCanResume, goalReasonLabel } from "@/features/goal/application/goalStatusPresentation";
import { type GoalReadModel } from "@/features/goal/application/goalReadModel";
function goal(patch: Partial<GoalReadModel> = {}): GoalReadModel {
  return {
    sessionId: "ses_1",
    objective: "Ship the retry fix",
    status: "active",
    used: { runs: 7, costUsd: 4.5, steps: 31 },
    createdAt: "2026-08-12T08:00:00Z",
    ...patch,
  };
}

describe("goal lifecycle actions", () => {
  it.each(["stoppedByUser", "awaitingInput", "blockedByModel"] as const)(
    "keeps %s resumable",
    (code) => {
      expect(
        goalCanResume(
          goal({
            status: code === "blockedByModel" ? "blocked" : "paused",
            reason: {
              code,
              ...(code === "blockedByModel" ? { explanation: "needs credentials" } : {}),
            },
          }),
        ),
      ).toBe(true);
    },
  );
});

describe("the wording tables", () => {
  it("refuses a runNotCompleted reason without an unsuccessful outcome", () => {
    expect(() => goalReasonLabel({ code: "runNotCompleted" })).toThrow("unsuccessful run outcome");
  });
});

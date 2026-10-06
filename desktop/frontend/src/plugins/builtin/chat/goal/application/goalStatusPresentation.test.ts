import { describe, expect, it } from "vitest";
import { en } from "@/lib/i18n/locales/en";
import { GOAL_STATUS_I18N, goalCanResume } from "./goalStatusPresentation";
import type { GoalReadModel } from "./goalReadModel";

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
          }),
        ),
      ).toBe(true);
    },
  );
});

describe("the wording tables", () => {
  it("names a catalog entry for every status", () => {
    const unworded = Object.values(GOAL_STATUS_I18N)
      .map((status) => status.label)
      .filter((key) => !(key in en));
    expect(unworded).toEqual([]);
  });
});

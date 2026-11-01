import { describe, expect, it } from "vitest";
import {
  runtimeGoalMaterial,
  toGoalCommandReceipt,
  toGoalReadModel,
} from "@/features/goal/adapters/runtimeGoalCommandsGateway";
describe("Runtime Goal Adapter", () => {
  it("projects the complete wire Goal into the neutral read model", () => {
    expect(
      toGoalReadModel({
        sessionId: "ses_goal",
        objective: "ship it",
        status: "blocked",
        reason: { code: "blockedByModel", explanation: "need an answer" },
        provider: "openai",
        model: "gpt-5",
        reasoningEffort: "high",
        used: { runs: 2, costUsd: 0.75, steps: 12 },
        createdAt: "2026-08-12T08:00:00Z",
        updatedAt: "2026-08-12T08:01:00Z",
      }),
    ).toEqual({
      sessionId: "ses_goal",
      objective: "ship it",
      status: "blocked",
      reason: { code: "blockedByModel", explanation: "need an answer" },
      used: { runs: 2, costUsd: 0.75, steps: 12 },
      createdAt: "2026-08-12T08:00:00Z",
    });
  });

  it("projects a mutation snapshot to a correlation receipt instead of standing state", () => {
    expect(
      toGoalCommandReceipt({
        sessionId: "ses_goal",
      }),
    ).toEqual({ sessionId: "ses_goal" });
  });

  it("retains the mounted Goal as read-only material when commands are unavailable", () => {
    const goal = {
      sessionId: "ses_goal",
      objective: "ship it",
      status: "active" as const,
      provider: "openai",
      model: "gpt-5",
      used: { runs: 0, costUsd: 0, steps: 0 },
      createdAt: "2026-08-12T08:00:00Z",
      updatedAt: "2026-08-12T08:01:00Z",
    };
    expect(runtimeGoalMaterial(goal, true)).toMatchObject({
      available: true,
      goal: { objective: "ship it" },
    });

    expect(runtimeGoalMaterial(goal, false)).toMatchObject({
      available: false,
      goal: { objective: "ship it" },
    });
  });
});

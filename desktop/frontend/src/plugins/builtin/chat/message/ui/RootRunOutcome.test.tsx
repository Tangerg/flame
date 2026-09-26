import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { CurrentRootMaterial } from "@/plugins/builtin/agent/public/run";
import type { AgentRunView } from "@/plugins/sdk/types/agentSessionView";
import { RootRunOutcome } from "./RootRunOutcome";

function completedRun(): AgentRunView {
  return {
    id: "run-completed",
    sessionId: "session-a",
    parentRunId: null,
    rootRunId: "run-completed",
    spawnedByItemId: null,
    status: "finished",
    activeSegmentId: null,
    outcome: { type: "completed" },
    metrics: {
      steps: 12,
      activeDurationMillis: 246_000,
      usage: { inputTokens: 82_400, outputTokens: 1_200, cacheReadTokens: 0, costUsd: 0.14 },
    },
    progress: null,
    contextTokens: null,
    createdAt: "2026-08-19T00:00:00Z",
    finishedAt: "2026-08-19T00:04:06Z",
  };
}

describe("RootRunOutcome", () => {
  afterEach(cleanup);

  it.each(["canceled", "failed", "timedOut", "lost", "completed"] as const)(
    "shows unresolved evidence independently of the %s outcome",
    (type) => {
      const run = completedRun();
      const unresolvedEffects = [
        {
          processId: "process-external",
          effectId: "effect-with-no-receipt",
          cause: "unknown",
          reason: "response_lost",
          detail: "connection closed before a response",
        },
      ];
      run.outcome = { type, error: { code: "run_lost" }, unresolvedEffects };
      const material = CurrentRootMaterial.from(run);
      const { rerender } = render(<RootRunOutcome material={material} />);
      const details = screen.getByRole("button", { name: "Operations with unconfirmed outcomes" });
      expect(screen.queryByText(/effect-with-no-receipt/)).toBeNull();
      fireEvent.click(details);
      expect(screen.getByText(/effect-with-no-receipt/).textContent).toBe(
        JSON.stringify(unresolvedEffects, null, 2),
      );
      rerender(<RootRunOutcome material={material} />);
      expect(
        screen.getAllByRole("button", { name: "Operations with unconfirmed outcomes" }),
      ).toHaveLength(1);
    },
  );

  it("does not append a completion-and-accounting footer after an ordinary turn", () => {
    const { container } = render(
      <RootRunOutcome material={CurrentRootMaterial.from(completedRun())} />,
    );

    expect(container.firstChild).toBeNull();
    expect(screen.queryByText("Completed")).toBeNull();
    expect(screen.queryByText(/82\.4k/)).toBeNull();
  });
});

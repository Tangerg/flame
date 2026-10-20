import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { CurrentRootMaterial } from "@/features/agent/public/run";
import { type AgentRunView } from "@/features/agent/public/sessionView";
import { RootRunOutcome } from "@/features/chat/message/ui/RootRunOutcome";
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
      run.outcome =
        type === "failed" || type === "timedOut" || type === "lost"
          ? { type, error: { code: "run_lost" }, unresolvedEffects }
          : { type, unresolvedEffects };
      const material = CurrentRootMaterial.from(run);
      const { rerender } = render(<RootRunOutcome material={material} />);
      expect(
        screen.getByText(
          "This run has ended. The outcomes of these external operations are still unconfirmed.",
        ),
      ).toBeTruthy();
      const details = screen.getByRole("button", { name: "Operations with unconfirmed outcomes" });
      expect(screen.queryByText(/effect-with-no-receipt/)).toBeNull();
      fireEvent.click(details);
      expect(details.getAttribute("aria-expanded")).toBe("true");
      expect(screen.getByText("effect-with-no-receipt")).toBeTruthy();
      expect(screen.getByText("process-external")).toBeTruthy();
      expect(screen.getByText("response_lost")).toBeTruthy();
      expect(screen.getByText("connection closed before a response")).toBeTruthy();
      expect(screen.queryByRole("button", { name: /retry|resume/i })).toBeNull();
      expect(material.outcome?.type).toBe(type);
      rerender(<RootRunOutcome material={material} />);
      expect(
        screen.getAllByRole("button", { name: "Operations with unconfirmed outcomes" }),
      ).toHaveLength(1);
    },
  );

  it("keeps current owner evidence exact across remounts", () => {
    const output = '{"receipt":9007199254740993,"text":"<script>unexpected()</script>"}';
    const run: AgentRunView = {
      ...completedRun(),
      outcome: {
        type: "canceled",
        unresolvedEffects: [
          {
            processId: "process-a",
            effectId: "effect-a",
            cause: "unknown",
            detail: "No final settlement was observed.",
            output,
          },
          { processId: "process-b", effectId: "effect-b", cause: "unknown" },
        ],
      },
    };
    const original = structuredClone(run);
    for (let mount = 0; mount < 2; mount += 1) {
      const { container, unmount } = render(
        <RootRunOutcome material={CurrentRootMaterial.from(run)} />,
      );
      expect(screen.getByText("Canceled")).toBeTruthy();
      expect(screen.getByText(/This run has ended/)).toBeTruthy();
      fireEvent.click(screen.getByRole("button", { name: "Operations with unconfirmed outcomes" }));
      expect(screen.getByText("effect-a")).toBeTruthy();
      expect(screen.getByText("effect-b")).toBeTruthy();
      expect(screen.queryByText("Reason code")).toBeNull();
      expect(
        screen.getByRole("region", { name: "Observed output (outcome unconfirmed)" }).textContent,
      ).toBe(output);
      expect(container.querySelector("script")).toBeNull();
      unmount();
    }
    expect(run).toEqual(original);
  });

  it("does not append a completion-and-accounting footer after an ordinary turn", () => {
    const { container } = render(
      <RootRunOutcome material={CurrentRootMaterial.from(completedRun())} />,
    );

    expect(container.firstChild).toBeNull();
    expect(screen.queryByText("Completed")).toBeNull();
    expect(screen.queryByText(/82\.4k/)).toBeNull();
  });
});

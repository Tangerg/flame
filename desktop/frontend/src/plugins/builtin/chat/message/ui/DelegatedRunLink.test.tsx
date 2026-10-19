import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { AgentRunView } from "@/plugins/sdk/types/agentSessionView";
import { DelegatedRunLink } from "./DelegatedRunLink";

const actions = vi.hoisted(() => ({
  open: vi.fn(),
  cancel: vi.fn(),
  selected: null as string | null,
}));
vi.mock("@/plugins/builtin/workspace/public/navigation", () => ({
  openWorkspaceSubagentRun: actions.open,
  useWorkspaceSubagentRunId: () => actions.selected,
}));
vi.mock("@/plugins/builtin/agent/public/run", () => ({ cancelSessionRun: actions.cancel }));
vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  useRuntimeCommandsAvailable: () => true,
}));

function run(overrides: Partial<AgentRunView> = {}): AgentRunView {
  return {
    id: "child-run",
    sessionId: "session-1",
    parentRunId: "root-run",
    rootRunId: "root-run",
    spawnedByItemId: "task-item",
    status: "running",
    activeSegmentId: "segment-1",
    outcome: null,
    metrics: {
      steps: 2,
      activeDurationMillis: 10,
      usage: { inputTokens: 3, outputTokens: 1, cacheReadTokens: 0 },
    },
    progress: { step: 3, activity: "Reviewing tests" },
    contextTokens: null,
    createdAt: "2026-01-01T00:00:00.000Z",
    finishedAt: null,
    ...overrides,
  };
}

describe("DelegatedRunLink", () => {
  it("opens the exact child in the dock and keeps waiting status visible", () => {
    render(
      <DelegatedRunLink
        run={run({ status: "waiting", activeSegmentId: null })}
        ordinal={1}
        siblingCount={1}
      />,
    );
    const opener = screen.getByRole("button", { name: /Sub-agent/ });
    fireEvent.click(opener);
    expect(actions.open).toHaveBeenLastCalledWith("child-run", opener);
    expect(screen.getByText("Needs input")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Cancel this run" }));
    expect(actions.cancel).toHaveBeenCalledWith({ sessionId: "session-1", runId: "child-run" });
  });

  it("reflects authoritative completion without retaining a cancel action", () => {
    const { rerender } = render(<DelegatedRunLink run={run()} ordinal={1} siblingCount={1} />);
    expect(screen.getByRole("button", { name: /Sub-agent/ }).hasAttribute("aria-current")).toBe(
      false,
    );
    actions.selected = "child-run";
    rerender(
      <DelegatedRunLink
        run={run({ status: "finished", activeSegmentId: null, outcome: { type: "completed" } })}
        ordinal={1}
        siblingCount={1}
      />,
    );
    expect(screen.getByText("Finished")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Cancel this run" })).toBeNull();
    const opener = screen.getByRole("button", { name: /Sub-agent/ });
    expect(opener.getAttribute("aria-current")).toBe("true");
    fireEvent.click(opener);
    expect(actions.open).toHaveBeenLastCalledWith("child-run", opener);
    actions.selected = null;
  });
});

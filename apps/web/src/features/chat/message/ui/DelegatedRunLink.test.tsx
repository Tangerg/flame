import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { fireEvent, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { type AgentRunView } from "@/features/agent/public/sessionView";
import { DelegatedRunLink } from "@/features/chat/message/ui/DelegatedRunLink";
const actions = vi.hoisted(() => ({
  open: vi.fn(),
  cancel: vi.fn(),
  selected: null as string | null,
}));

vi.mock("@/features/workspace/public/navigation", () => ({
  openWorkspaceSubagentRun: actions.open,
  useWorkspaceSubagentRunId: () => actions.selected,
}));

prepareTestClient(() => {
  Object.assign(testClient.agent.access, { cancelSessionRun: actions.cancel });
});

vi.mock("@/features/connection/public/serviceStatus", () => ({
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
    testClient.render(
      <DelegatedRunLink
        run={run({ status: "waiting", activeSegmentId: null })}
        ordinal={1}
        siblingCount={1}
      />,
    );
    const opener = screen.getByRole("button", { name: /Sub-agent/ });
    fireEvent.click(opener);
    expect(actions.open).toHaveBeenLastCalledWith(testClient.workspace.access, "child-run", opener);
    expect(screen.getByText("Needs input")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Cancel this run" }));
    expect(actions.cancel).toHaveBeenCalledWith({
      sessionId: "session-1",
      runId: "child-run",
    });
  });

  it("reflects authoritative completion without retaining a cancel action", () => {
    const { rerender } = testClient.render(
      <DelegatedRunLink run={run()} ordinal={1} siblingCount={1} />,
    );
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
    expect(actions.open).toHaveBeenLastCalledWith(testClient.workspace.access, "child-run", opener);
    actions.selected = null;
  });
});

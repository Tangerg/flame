import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import type { TranscriptRow } from "@/plugins/builtin/agent/public/conversation";
import type { AgentRunView } from "@/plugins/sdk/types/agentSessionView";
import { navigator } from "@/lib/navigation";
import { openWorkspaceSubagentRun } from "@/plugins/builtin/workspace/public/navigation";
import { SubagentsPanel } from "./SubagentsPanel";

const material = vi.hoisted(() => ({ rows: [] as readonly TranscriptRow[] }));
vi.mock("@/plugins/builtin/agent/public/conversation", () => ({
  useActiveConversationRows: () => material.rows,
}));
vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  useRuntimeCommandsAvailable: () => true,
}));

const child: AgentRunView = {
  id: "child",
  sessionId: "session",
  parentRunId: "root",
  rootRunId: "root",
  spawnedByItemId: "delegate",
  status: "running",
  activeSegmentId: "segment",
  outcome: null,
  progress: { step: 1, activity: "Inspecting axios" },
  contextTokens: null,
  metrics: {
    steps: 1,
    activeDurationMillis: 10,
    usage: { inputTokens: 1, outputTokens: 1, cacheReadTokens: 0 },
  },
  createdAt: "2026-01-01T00:00:00Z",
  finishedAt: null,
};

function rows(run = child, text = "Child streamed output"): TranscriptRow[] {
  return [
    {
      message: { id: "parent", role: "assistant", runId: "root", blocks: [] },
      runOwner: { kind: "owned", runId: "root", status: "running" },
      facts: {
        toolCalls: {},
        delegatedRuns: {
          delegate: [
            {
              run,
              messages: [
                {
                  id: "child-output",
                  role: "assistant",
                  runId: run.id,
                  phase: "commentary",
                  blocks: [{ kind: "text", text, status: "complete" }],
                },
              ],
            },
          ],
          nested: [
            {
              run: {
                ...child,
                id: "nested",
                parentRunId: "child",
                spawnedByItemId: "nested",
                status: "finished",
                activeSegmentId: null,
                outcome: { type: "completed" },
              },
              messages: [],
            },
          ],
        },
      },
    },
  ];
}

it("projects live child material, returns to the list, and never carries it into another session", () => {
  material.rows = rows();
  navigator().go({ session: "session" });
  openWorkspaceSubagentRun("child");
  const { rerender } = render(<SubagentsPanel />);
  expect(screen.getByText("Child streamed output")).toBeTruthy();
  expect(screen.getByText("Running")).toBeTruthy();

  material.rows = rows(
    { ...child, status: "finished", activeSegmentId: null, outcome: { type: "completed" } },
    "Child final answer",
  );
  rerender(<SubagentsPanel />);
  expect(screen.queryByText("Child streamed output")).toBeNull();
  expect(screen.getByText("Child final answer")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Cancel this run" })).toBeNull();

  fireEvent.click(screen.getByRole("button", { name: "Subagents" }));
  expect(navigator().get()).toMatchObject({
    session: "session",
    dock: "subagents",
    subagent: null,
  });
  expect(screen.getByText("Completed · 2")).toBeTruthy();
  expect(screen.queryByText("Child final answer")).toBeNull();
  const list = screen.getByRole("region", { name: "Subagents" });
  fireEvent.click(within(list).getAllByRole("button", { name: /Sub-agent/ })[1]!);
  expect(navigator().get().subagent).toBe("nested");
  expect(screen.getByText("No narrative material yet.")).toBeTruthy();

  material.rows = [];
  act(() => navigator().go({ session: "other" }));
  rerender(<SubagentsPanel />);
  expect(screen.getByText("No subagents yet")).toBeTruthy();
  expect(navigator().get().subagent).toBeNull();
});

import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import type { TranscriptRow } from "@/plugins/builtin/agent/public/conversation";
import type { AgentRunView } from "@/plugins/sdk/types/agentSessionView";
import { navigator } from "@/lib/navigation";
import { openWorkspaceSubagentRun } from "@/plugins/builtin/workspace/public/navigation";
import { SubagentsPanel } from "./SubagentsPanel";

const material = vi.hoisted(() => ({
  rows: [] as readonly TranscriptRow[],
  available: true,
  cancel: vi.fn(),
}));
vi.mock("@/plugins/builtin/agent/public/run", () => ({ cancelSessionRun: material.cancel }));
vi.mock("@/plugins/builtin/agent/public/conversation", () => ({
  useActiveConversationRows: () => material.rows,
}));
vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  useRuntimeCommandsAvailable: () => material.available,
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
  expect(screen.getByText("No narrative material yet")).toBeTruthy();

  material.rows = [];
  act(() => navigator().go({ session: "other" }));
  rerender(<SubagentsPanel />);
  expect(screen.getByText("No subagents yet")).toBeTruthy();
  expect(navigator().get().subagent).toBeNull();
});

// The corner and fill audits find speech by this marker. A delegated user message went
// unmarked and so wore a bubble nobody was comparing — a third radius and padding, stacked
// over a second one, against the transcript's.
it("marks a delegated user message as the same speech bubble the transcript draws", () => {
  material.rows = rows();
  material.rows[0]!.facts.delegatedRuns.delegate![0]!.messages.unshift({
    id: "child-task",
    role: "user",
    runId: "child",
    blocks: [{ kind: "text", text: "Audit axios cancellation", status: "complete" }],
  });
  navigator().go({ session: "session" });
  openWorkspaceSubagentRun("child");
  const { container } = render(<SubagentsPanel />);

  const bubbles = container.querySelectorAll("[data-user-message-bubble]");
  expect(bubbles).toHaveLength(1);
  expect(bubbles[0]!.textContent).toContain("Audit axios cancellation");
  expect(
    screen.getByText("Child streamed output").closest("[data-user-message-bubble]"),
  ).toBeNull();
});

it("keeps the selected run failure visible when no narrative was produced", () => {
  material.rows = rows({
    ...child,
    status: "finished",
    activeSegmentId: null,
    outcome: { type: "maxBudget", detail: "The review exhausted its token budget." },
  });
  material.rows[0]!.facts.delegatedRuns.delegate![0]!.messages = [];
  navigator().go({ session: "session" });
  openWorkspaceSubagentRun("child");
  render(<SubagentsPanel />);
  expect(screen.getByText("The review exhausted its token budget.")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Cancel this run" })).toBeNull();
});

it("cancels the currently selected child and disables mutations while disconnected", () => {
  material.rows = rows();
  material.available = true;
  navigator().go({ session: "session" });
  openWorkspaceSubagentRun("child");
  const { rerender } = render(<SubagentsPanel />);
  fireEvent.click(screen.getByRole("button", { name: "Cancel this run" }));
  expect(material.cancel).toHaveBeenLastCalledWith({ sessionId: "session", runId: "child" });
  const nested = material.rows[0]!.facts.delegatedRuns.nested![0]!;
  nested.run = { ...child, id: "nested", parentRunId: "child", spawnedByItemId: "nested" };
  act(() => openWorkspaceSubagentRun("nested"));
  fireEvent.click(screen.getByRole("button", { name: "Cancel this run" }));
  expect(material.cancel).toHaveBeenLastCalledWith({ sessionId: "session", runId: "nested" });
  material.available = false;
  rerender(<SubagentsPanel />);
  expect(screen.getByRole("button", { name: "Cancel this run" }).hasAttribute("disabled")).toBe(
    true,
  );
  material.available = true;
});

it("names delegated tasks from their parent tool call in both list and detail", () => {
  material.rows = rows();
  material.rows[0]!.facts.toolCalls.delegate = {
    id: "delegate",
    runId: "root",
    name: "delegate_task",
    fn: "Audit axios cancellation",
    args: '{"summary":"Audit axios cancellation"}',
    status: "ok",
  };
  material.rows[0]!.facts.toolCalls.nested = {
    id: "nested",
    runId: "child",
    name: "delegate_task",
    fn: "Review request headers",
    args: '{"summary":"Review request headers"}',
    status: "ok",
  };
  navigator().go({ session: "session" });
  openWorkspaceSubagentRun(null);
  render(<SubagentsPanel />);
  expect(screen.getByRole("button", { name: /Review request headers/ })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: /Audit axios cancellation/ }));
  expect(screen.getByRole("region", { name: "Audit axios cancellation" })).toBeTruthy();
  expect(navigator().get().subagent).toBe("child");
});

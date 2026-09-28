import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import type { AgentRunFact } from "@/plugins/sdk";
import type { TrajectoryEntry } from "@/plugins/builtin/agent/public/run";

const projection = vi.hoisted(() => ({
  runtimeAvailable: false,
  includeDescendants: true,
  sessionId: "session_one",
  entries: [] as TrajectoryEntry[],
  nextCursor: undefined as string | undefined,
  read: vi.fn(),
  runContext: vi.fn(),
  refetch: vi.fn(),
  openSubagent: vi.fn(),
  locateTool: vi.fn(),
  cancelRun: vi.fn(),
  exportTrajectory: vi.fn(),
}));

const run: AgentRunFact = {
  id: "run_one",
  sessionId: "session_one",
  parentRunId: null,
  rootRunId: "run_one",
  spawnedByItemId: null,
  status: "running",
  activeSegmentId: "segment_one",
  outcome: null,
  modelSelection: { provider: "deepseek", model: "deepseek-chat", reasoningEffort: "high" },
  metrics: {
    steps: 2,
    activeDurationMillis: 10,
    usage: { inputTokens: 1, outputTokens: 1, cacheReadTokens: 0 },
  },
  createdAt: "2026-09-14T01:00:00Z",
  finishedAt: null,
};

vi.mock("@/plugins/builtin/agent/public/run", () => ({
  cancelSessionRun: projection.cancelRun,
  useSessionTrajectory: projection.read,
  useTrajectoryRun: projection.runContext,
}));
vi.mock("@/plugins/builtin/agent/public/session", () => ({
  useActiveSessionId: () => projection.sessionId,
  useActiveSession: () => ({ id: projection.sessionId, status: "idle" }),
}));
vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  useRuntimeCommandsAvailable: () => projection.runtimeAvailable,
}));
vi.mock("@/plugins/builtin/runtime/public/capabilities", () => ({
  useRuntimeCapability: (capability: string) =>
    capability === "subagents" ? projection.includeDescendants : true,
}));
vi.mock("@/plugins/builtin/workspace/public/navigation", () => ({
  locateWorkspaceTool: projection.locateTool,
  openWorkspaceSubagentRun: projection.openSubagent,
}));
vi.mock("@/plugins/builtin/workspace/public/conversationArchive", () => ({
  exportSessionTrajectory: projection.exportTrajectory,
}));
vi.mock("../WorkspaceViewLayout", () => ({
  WorkspaceViewLayout: ({
    children,
    sub,
    actions,
  }: {
    children: ReactNode;
    sub: ReactNode;
    actions: ReactNode;
  }) => (
    <div>
      {sub}
      {actions}
      {children}
    </div>
  ),
}));
import { Timeline } from "./Timeline";

describe("durable timeline", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    projection.entries = [];
    projection.nextCursor = undefined;
    projection.sessionId = "session_one";
    projection.runtimeAvailable = false;
    projection.includeDescendants = true;
    projection.read.mockImplementation((_sessionId, _includeDescendants, cursor) => ({
      data: cursor ? { data: [] } : { data: projection.entries, nextCursor: projection.nextCursor },
      isLoading: false,
      isFetching: false,
      error: null,
      refetch: projection.refetch,
    }));
    projection.runContext.mockReturnValue({
      data: run,
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    projection.exportTrajectory.mockResolvedValue(undefined);
  });

  it("unifies model and tool records, exposes evidence, and keeps unmeasured outcomes unknown", () => {
    projection.entries = [
      {
        type: "model",
        occurredAt: run.createdAt,
        model: {
          callId: "call_unknown",
          runId: run.id,
          segmentId: "segment_one",
          state: "unknown",
          startedAt: run.createdAt,
          settledAt: "2026-09-14T02:00:00Z",
        },
      },
      {
        type: "item",
        occurredAt: run.createdAt,
        item: {
          type: "toolCall",
          id: "tool_zero",
          runId: run.id,
          startedAt: run.createdAt,
          status: "completed",
          durationMillis: 0,
          tool: {
            name: "shell",
            arguments: { command: "rg axios", description: "Find axios" },
            result: { exitCode: 1, stdout: "No matches" },
          },
        },
      },
    ];
    render(<Timeline />);
    expect(screen.getByText("This page: 2 / 2")).toBeTruthy();
    expect(screen.getByText("Outcome unknown")).toBeTruthy();
    expect(screen.getByText("rg axios")).toBeTruthy();
    expect(screen.getByText("0ms")).toBeTruthy();
    expect(screen.queryByText("1h 00m")).toBeNull();
    expect(projection.runContext).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Inspect call_unknown" }));
    expect(screen.getByText("Recorded settlement")).toBeTruthy();
    expect(screen.getByText("2026-09-14T02:00:00Z")).toBeTruthy();
    expect(
      screen.getByText(
        "The final outcome was not observed. Settlement time is not measured execution time.",
      ),
    ).toBeTruthy();
    expect(screen.getByText("deepseek/deepseek-chat")).toBeTruthy();
    expect(projection.runContext).toHaveBeenCalledWith(run.id);
    fireEvent.click(screen.getByRole("button", { name: "Inspect tool_zero" }));
    expect(screen.getByText("Arguments")).toBeTruthy();
    expect(screen.getByText("Result")).toBeTruthy();
    expect(screen.getAllByText("Completed")).toHaveLength(1);
  });

  it("shows zero usage, first output timing, and the assistant message phase", () => {
    projection.entries = [
      {
        type: "model",
        occurredAt: run.createdAt,
        model: {
          callId: "call_done",
          runId: run.id,
          segmentId: "segment_one",
          state: "completed",
          startedAt: run.createdAt,
          settledAt: "2026-09-14T01:00:02Z",
          firstOutputLatencyMillis: 0,
          usage: {
            inputTokens: 123,
            outputTokens: 0,
            cacheReadTokens: 31,
            cacheWriteTokens: 0,
            reasoningTokens: 0,
          },
        },
      },
      {
        type: "item",
        occurredAt: run.createdAt,
        item: {
          type: "agentMessage",
          id: "answer",
          runId: run.id,
          createdAt: run.createdAt,
          status: "completed",
          phase: "finalAnswer",
          content: [{ type: "text", text: "All done" }],
        },
      },
    ];
    render(<Timeline />);
    fireEvent.click(screen.getByRole("button", { name: "Inspect call_done" }));
    expect(screen.getByText("First output")).toBeTruthy();
    expect(screen.getByText("0ms")).toBeTruthy();
    expect(screen.getByText("123")).toBeTruthy();
    expect(screen.getByText("31")).toBeTruthy();
    expect(screen.getByText("Output tokens").nextElementSibling?.textContent).toBe("0");
    expect(screen.getByText("cache write").nextElementSibling?.textContent).toBe("0");
    expect(screen.getByText("reasoning").nextElementSibling?.textContent).toBe("0");
    fireEvent.click(screen.getByRole("button", { name: "Inspect answer" }));
    expect(screen.getByText("Final answer")).toBeTruthy();
  });

  it("reveals retained image evidence only when its record is opened", () => {
    projection.entries = [
      {
        type: "item",
        occurredAt: run.createdAt,
        item: {
          type: "userMessage",
          id: "image_input",
          runId: run.id,
          createdAt: run.createdAt,
          status: "completed",
          content: [{ type: "image", mime: "image/png", data: "aW1hZ2U=" }],
        },
      },
    ];
    render(<Timeline />);
    expect(screen.queryByRole("img", { name: "image/png" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Inspect image_input" }));
    expect(screen.getByRole("img", { name: "image/png" }).getAttribute("src")).toBe(
      "data:image/png;base64,aW1hZ2U=",
    );
  });

  it("filters current-page evidence and pages only on request", () => {
    projection.nextCursor = "older";
    projection.entries = [
      {
        type: "item",
        occurredAt: run.createdAt,
        item: {
          type: "toolCall",
          id: "tool_search",
          runId: run.id,
          startedAt: run.createdAt,
          status: "completed",
          tool: {
            name: "shell",
            arguments: { command: "rg needle" },
            result: "searchable evidence",
          },
        },
      },
    ];
    const { rerender } = render(<Timeline />);
    expect(projection.read).toHaveBeenLastCalledWith("session_one", true, undefined);
    fireEvent.change(screen.getByRole("searchbox"), { target: { value: "missing" } });
    expect(screen.getByText("This page: 0 / 1")).toBeTruthy();
    expect(screen.getByText("No matching records on this page")).toBeTruthy();
    fireEvent.change(screen.getByRole("searchbox"), { target: { value: "searchable evidence" } });
    expect(screen.getByRole("button", { name: "Inspect tool_search" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Older records" }));
    expect(projection.read).toHaveBeenLastCalledWith("session_one", true, "older");
    fireEvent.click(screen.getByRole("button", { name: "Newer records" }));
    expect(projection.read).toHaveBeenLastCalledWith("session_one", true, undefined);
    fireEvent.click(screen.getByRole("button", { name: "Older records" }));
    projection.sessionId = "session_two";
    rerender(<Timeline />);
    expect(projection.read).toHaveBeenLastCalledWith("session_two", true, undefined);
    expect((screen.getByRole("searchbox") as HTMLInputElement).value).toBe("");
    fireEvent.click(screen.getByRole("button", { name: "Older records" }));
    projection.includeDescendants = false;
    rerender(<Timeline />);
    expect(projection.read).toHaveBeenLastCalledWith("session_two", false, undefined);
    expect(
      projection.read.mock.calls
        .filter(([, descendants]) => descendants === false)
        .every(([, , cursor]) => cursor === undefined),
    ).toBe(true);
  });

  it.each([
    ["run_one", null],
    ["child_parent", "child_parent"],
  ])("locates the task's owning parent %s while offline", (parentRunId, expectedChild) => {
    projection.entries = [
      {
        type: "run",
        occurredAt: run.createdAt,
        run: { ...run, id: "child", parentRunId, spawnedByItemId: "parent_task" },
      },
    ];
    render(<Timeline />);
    fireEvent.click(screen.getByRole("button", { name: "Locate parent task" }));
    expect(projection.openSubagent.mock.calls).toEqual(expectedChild ? [[expectedChild]] : []);
    expect(projection.locateTool.mock.calls).toEqual(expectedChild ? [] : [["parent_task"]]);
  });

  it("gates mutations offline and exports complete history independently of page filters", async () => {
    projection.entries = [{ type: "run", occurredAt: run.createdAt, run }];
    const { rerender } = render(<Timeline />);
    expect(
      (screen.getByRole("button", { name: "Cancel this run" }) as HTMLButtonElement).disabled,
    ).toBe(true);
    expect(
      (screen.getByRole("button", { name: "Export complete trajectory" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    projection.runtimeAvailable = true;
    rerender(<Timeline />);
    fireEvent.change(screen.getByRole("searchbox"), { target: { value: "missing" } });
    fireEvent.click(screen.getByRole("button", { name: "Export complete trajectory" }));
    await waitFor(() => expect(projection.exportTrajectory).toHaveBeenCalledTimes(1));
    expect(projection.cancelRun).not.toHaveBeenCalled();
  });
});

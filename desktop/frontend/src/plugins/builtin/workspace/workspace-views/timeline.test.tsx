import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import type { AgentRunView, TimelineEntry, ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import { lookupExtensionPoint } from "@/plugins/sdk/selectors/extensions";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

const projection = vi.hoisted(() => ({
  runtimeAvailable: false,
  child: null as AgentRunView | null,
  openSubagent: vi.fn(),
  locateTool: vi.fn(),
  cancelRun: vi.fn(),
  timeline: [] as TimelineEntry[],
  tools: {} as Record<string, ToolCall>,
}));

const running: AgentRunView = {
  id: "run-1",
  sessionId: "session-1",
  parentRunId: null,
  rootRunId: "run-1",
  spawnedByItemId: null,
  status: "running",
  activeSegmentId: "segment-1",
  outcome: null,
  metrics: {
    steps: 2,
    activeDurationMillis: 10,
    usage: { inputTokens: 1, outputTokens: 1, cacheReadTokens: 0 },
  },
  progress: { step: 3, activity: "Inspecting" },
  contextTokens: null,
  createdAt: "2026-01-01T00:00:00.000Z",
  finishedAt: null,
};

vi.mock("@/plugins/builtin/agent/public/run", () => ({
  cancelSessionRun: projection.cancelRun,
  useActiveSessionRunTree: () => [
    { run: running, children: projection.child ? [{ run: projection.child, children: [] }] : [] },
  ],
  useActiveSessionTimeline: () => projection.timeline,
  useActiveSessionToolCalls: () => projection.tools,
}));

vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  useRuntimeCommandsAvailable: () => projection.runtimeAvailable,
}));

vi.mock("@/plugins/builtin/workspace/public/navigation", () => ({
  locateWorkspaceTool: projection.locateTool,
  openWorkspaceSubagentRun: projection.openSubagent,
  selectWorkspaceChat: vi.fn(),
}));

vi.mock("./views/WorkspaceViewLayout", () => ({
  WorkspaceViewLayout: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

import { timelineView } from "./index";
import { TimelineTab } from "./timeline";

describe("Timeline runtime actions", () => {
  beforeEach(() => {
    projection.child = null;
    projection.runtimeAvailable = false;
    projection.openSubagent.mockClear();
    projection.locateTool.mockClear();
    projection.timeline = [];
    projection.tools = {};
  });

  it("shows measured execution time and keeps absent timing unknown", () => {
    projection.tools = {
      measured: {
        id: "measured",
        runId: running.id,
        name: "shell",
        fn: "Verify axios",
        args: "",
        status: "ok",
        durationMillis: 230,
      },
      unknown: {
        id: "unknown",
        runId: running.id,
        name: "shell",
        fn: "Inspect axios",
        args: "",
        status: "ok",
      },
    };
    projection.timeline = [
      {
        id: "measured-end",
        runId: running.id,
        refId: "measured",
        kind: "tool",
        status: "ok",
        ts: 1,
      },
      { id: "unknown-end", runId: running.id, refId: "unknown", kind: "tool", status: "ok", ts: 2 },
      { id: "compact", runId: running.id, refId: "item_compact", kind: "compaction", ts: 3 },
    ];
    render(<TimelineTab />);
    expect(screen.getByText("230ms")).toBeTruthy();
    expect(screen.getByText("—")).toBeTruthy();
    expect(screen.getByText("Verify axios")).toBeTruthy();
    expect(screen.getByText("Context compacted")).toBeTruthy();
  });
  it("shows outcome details only on completion and does not classify a nonzero exit as failure", () => {
    projection.tools = {
      failed: {
        id: "failed",
        runId: running.id,
        name: "shell",
        fn: "Verify axios",
        args: "",
        status: "err",
        error: "request canceled while reading the response body",
        exitCode: 2,
      },
      noMatch: {
        id: "noMatch",
        runId: running.id,
        name: "shell",
        fn: "Find axios TODOs",
        args: "",
        status: "ok",
        exitCode: 1,
      },
    };
    projection.timeline = [
      {
        id: "failed-end",
        runId: running.id,
        refId: "failed",
        kind: "tool",
        ts: 2,
        status: "err",
      },
      {
        id: "no-match-end",
        runId: running.id,
        refId: "noMatch",
        kind: "tool",
        ts: 3,
        status: "ok",
      },
    ];
    render(<TimelineTab />);
    expect(screen.getAllByText("request canceled while reading the response body")).toHaveLength(1);
    expect(screen.getAllByText("exit 2")).toHaveLength(1);
    expect(screen.getByText("exit 1")).toBeTruthy();
    expect(screen.getAllByRole("img", { name: "err" })).toHaveLength(1);
    expect(screen.getByRole("img", { name: "ok" })).toBeTruthy();
  });
  it.each([
    ["run-1", null],
    ["child-parent", "child-parent"],
  ])(
    "opens the retained transcript that owns parent %s while offline",
    (parentRunId, subagentRunId) => {
      projection.child = {
        ...running,
        id: "child",
        rootRunId: running.id,
        parentRunId,
        spawnedByItemId: "parent-delegation",
      };
      render(<TimelineTab />);
      fireEvent.click(screen.getByRole("button", { name: "Locate parent task" }));
      expect(projection.openSubagent.mock.calls).toEqual(subagentRunId ? [[subagentRunId]] : []);
      expect(projection.locateTool.mock.calls).toEqual(
        subagentRunId ? [] : [["parent-delegation"]],
      );
    },
  );
  it("does not offer an active cancel command while the Runtime is unavailable", async () => {
    await loadPluginsForTest(timelineView);
    expect(lookupExtensionPoint(WORKSPACE_VIEW).some((view) => view.id === "timeline")).toBe(true);

    render(<TimelineTab />);

    const cancel = screen.getByRole("button", { name: "Cancel this run" });
    expect((cancel as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(cancel);
    expect(projection.cancelRun).not.toHaveBeenCalled();
  });
});

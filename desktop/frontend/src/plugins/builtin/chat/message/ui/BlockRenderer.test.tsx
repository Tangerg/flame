import { navigator } from "@/lib/navigation";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { AgentRunView, Message, ToolCall } from "@/plugins/sdk/types/agentSessionView";
import type { TurnFacts } from "@/plugins/builtin/agent/public/conversation";
import { MessageContext } from "@/plugins/sdk/messageContext";
import type { BlockCtx } from "./BlockRenderer";
import { renderBlock, renderMessageBlocks } from "./BlockRenderer";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_STANDING_SURFACE } from "@/plugins/sdk/kernelPoints";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

const CTX: BlockCtx = {
  expandedIds: new Set(),
  onToggleExpand: vi.fn(),
  textReveal: "smooth",
};

const agentRunCommands = vi.hoisted(() => ({ cancel: vi.fn() }));
vi.mock("@/plugins/builtin/agent/public/run", () => ({
  cancelSessionRun: agentRunCommands.cancel,
}));
vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  useRuntimeCommandsAvailable: () => true,
}));

function run(
  id: string,
  parentRunId: string,
  rootRunId: string,
  spawnedByItemId: string,
  status: AgentRunView["status"] = "finished",
): AgentRunView {
  return {
    id,
    sessionId: "session-1",
    parentRunId,
    rootRunId,
    spawnedByItemId,
    status,
    activeSegmentId: status === "running" ? `segment-${id}` : null,
    outcome: status === "finished" ? { type: "completed" } : null,
    metrics: {
      steps: 1,
      activeDurationMillis: 1,
      usage: { inputTokens: 1, outputTokens: 1, cacheReadTokens: 0 },
    },
    progress: null,
    contextTokens: null,
    createdAt: "2026-01-01T00:00:00.000Z",
    finishedAt: status === "finished" ? "2026-01-01T00:00:01.000Z" : null,
  };
}

function tool(id: string): ToolCall {
  return {
    id,
    runId: "root-run",
    name: "task",
    fn: "Delegate work",
    args: "{}",
    status: "ok",
  };
}

function message(id: string, runId: string, toolCallId: string): Message {
  return {
    id,
    runId,
    role: "assistant",
    blocks: [{ kind: "tool", toolCallId }],
  };
}

function renderRootTool(toolCallId: string, facts: TurnFacts) {
  const rootMessage = message("root-message", "root-run", toolCallId);
  return render(
    <MessageContext.Provider value={{ sessionId: "session-1", message: rootMessage }}>
      {renderBlock(rootMessage.blocks[0]!, 0, facts, CTX)}
    </MessageContext.Provider>,
  );
}

describe("delegated Run rendering", () => {
  it("opens a child in the dock without expanding descendants into its parent", () => {
    const parentTool = tool("task-root");
    const nestedTool = { ...tool("task-child"), runId: "child-run" };
    const facts: TurnFacts = {
      toolCalls: {
        [parentTool.id]: parentTool,
        [nestedTool.id]: nestedTool,
      },
      delegatedRuns: {
        [parentTool.id]: [
          {
            run: run("child-run", "root-run", "root-run", parentTool.id),
            messages: [message("child-message", "child-run", nestedTool.id)],
          },
        ],
        [nestedTool.id]: [
          {
            run: run("nested-run", "child-run", "root-run", nestedTool.id),
            messages: [],
          },
        ],
      },
    };

    renderRootTool(parentTool.id, facts);
    expect(screen.getAllByText("Sub-agent")).toHaveLength(1);
    expect(screen.queryByText("No narrative material yet.")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: /Sub-agent/ }));
    expect(navigator().get()).toMatchObject({ dock: "subagents", subagent: "child-run" });
    expect(screen.getAllByText("Sub-agent")).toHaveLength(1);

    const taskAnchors = document.querySelectorAll("#task-root, #task-child");
    expect(taskAnchors).toHaveLength(1);
  });

  it("targets the exact descendant Run when its stop action is used", () => {
    agentRunCommands.cancel.mockClear();
    const parentTool = tool("task-root");
    const facts: TurnFacts = {
      toolCalls: { [parentTool.id]: parentTool },
      delegatedRuns: {
        [parentTool.id]: [
          {
            run: run("child-run", "root-run", "root-run", parentTool.id, "running"),
            messages: [],
          },
        ],
      },
    };

    renderRootTool(parentTool.id, facts);
    fireEvent.click(screen.getByRole("button", { name: "Cancel this run" }));

    expect(agentRunCommands.cancel).toHaveBeenCalledOnce();
    expect(agentRunCommands.cancel).toHaveBeenCalledWith({
      sessionId: "session-1",
      runId: "child-run",
    });
  });
});

describe("standing tool outcomes", () => {
  it("keeps a failed Plan call visible after a successful retry", async () => {
    await loadPluginsForTest(
      definePlugin({
        name: "test.plan-surface",
        setup(ctx) {
          ctx.contribute(TOOL_STANDING_SURFACE, "plan", { key: "set_plan" });
        },
      }),
    );
    const call: ToolCall = {
      ...tool("plan-call"),
      name: "set_plan",
      fn: "Update plan",
      status: "running",
    };
    const row = {
      message: message("plan-message", "root-run", call.id),
      facts: { toolCalls: { [call.id]: call }, delegatedRuns: {} },
    };
    const renderRow = () => (
      <MessageContext.Provider value={{ sessionId: "session-1", message: row.message }}>
        {renderMessageBlocks(row, CTX)}
      </MessageContext.Provider>
    );
    const { rerender } = render(renderRow());
    expect(document.querySelector('[data-tool="set_plan"]')).not.toBeNull();

    const error = "at most one step may be in_progress";
    row.facts.toolCalls[call.id] = { ...call, status: "err", error };
    rerender(renderRow());
    expect(screen.getByText(error)).toBeTruthy();

    const retry = { ...call, id: "plan-retry", status: "ok" as const };
    row.facts.toolCalls[retry.id] = retry;
    row.message.blocks.push({ kind: "tool", toolCallId: retry.id });
    rerender(renderRow());

    expect(screen.getByText(error)).toBeTruthy();
    expect(document.querySelectorAll('[data-tool="set_plan"]')).toHaveLength(1);
  });
});

import { navigator } from "@/lib/navigation";
import { AnimatePresence } from "motion/react";
import { fireEvent, render, screen, within } from "@testing-library/react";
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

describe("approval settlement rendering", () => {
  it.each([
    ["approve", "Approved"],
    ["deny", "Declined"],
  ] as const)("keeps the Runtime's %s decision after the handoff closes", (decision, label) => {
    const id = "approval-tool";
    const block = {
      kind: "approval" as const,
      itemId: id,
      toolName: "shell",
      command: "pwd",
      reason: "Runs commands in the workspace.",
    };
    const facts: TurnFacts = {
      toolCalls: {
        [id]: { ...tool(id), name: "shell", fn: "pwd", status: "running" },
      },
      delegatedRuns: {},
      awaiting: new Map([[id, "root-run"]]),
    };
    const { rerender } = render(renderBlock(block, 0, facts, CTX));
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Allow once" }).disabled).toBe(
      false,
    );

    facts.awaiting = new Map();
    facts.toolCalls[id] = { ...facts.toolCalls[id]!, approvalDecision: decision };
    rerender(renderBlock(block, 0, facts, CTX));
    expect(screen.getByText(label, { exact: true })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Allow once" })).toBeNull();

    facts.toolCalls[id] = {
      ...facts.toolCalls[id]!,
      status: decision === "approve" ? "ok" : "denied",
    };
    rerender(renderBlock(block, 0, facts, CTX));
    expect(screen.getByText(label, { exact: true })).toBeTruthy();
  });

  it("does not leave a withdrawn approval as a disabled request", () => {
    const block = {
      kind: "approval" as const,
      itemId: "cancelled-tool",
      toolName: "shell",
      command: "pwd",
      reason: "Runs commands in the workspace.",
    };
    const facts: TurnFacts = {
      toolCalls: {
        [block.itemId]: { ...tool(block.itemId), status: "err", error: "cancelled" },
      },
      delegatedRuns: {},
      awaiting: new Map(),
    };
    const { container } = render(renderBlock(block, 0, facts, CTX));
    expect(container.querySelector('[data-slot="approval-surface"]')).toBeNull();
    expect(screen.queryByText("Approved", { exact: true })).toBeNull();
    expect(screen.queryByText("Declined", { exact: true })).toBeNull();
  });
});

it("animates opaque tool arrivals without remounting their surface when regrouped", () => {
  const first: ToolCall = {
    id: "read-first",
    runId: "root-run",
    name: "read",
    fn: "Read first file",
    args: "{}",
    safetyClass: "safe",
    status: "running",
  };
  const row = {
    message: { ...message("tool-arrivals", "root-run", first.id), blocks: [] as Message["blocks"] },
    facts: {
      toolCalls: {} as Record<string, ToolCall>,
      delegatedRuns: {},
      awaiting: new Map<string, string>(),
    },
  };
  const renderRow = () => (
    <MessageContext.Provider value={{ sessionId: "session-1", message: row.message }}>
      <AnimatePresence initial={false}>{renderMessageBlocks(row, CTX)}</AnimatePresence>
    </MessageContext.Provider>
  );
  const { container, rerender } = render(renderRow());
  const expectImmediate = () => {
    const units = container.querySelectorAll<HTMLElement>("[data-block-anchor]");
    expect(units.length).toBeGreaterThan(0);
    for (const unit of units) {
      const style = getComputedStyle(unit);
      expect(Number(style.opacity || "1")).toBe(1);
      expect(style.transform || "none").toBe("none");
    }
  };

  row.facts.toolCalls[first.id] = first;
  row.message.blocks.push({ kind: "tool", toolCallId: first.id });
  rerender(renderRow());
  expectImmediate();
  const surface = container.querySelector<HTMLElement>("[data-block-anchor]")!;
  expect(surface.style.top).toBe("4px");

  const second = { ...first, id: "read-second", fn: "Read second file" };
  row.facts.toolCalls[second.id] = second;
  row.message.blocks.push({ kind: "tool", toolCallId: second.id });
  rerender(renderRow());
  expectImmediate();
  expect(container.querySelector("[data-block-anchor]")).toBe(surface);

  row.message.blocks.push({
    kind: "reasoning",
    reasoningId: "compare-files",
    text: "Compare the files",
    status: "complete",
  });
  row.message.blocks.push({ kind: "text", text: "Both files agree.", status: "complete" });
  rerender(renderRow());
  expectImmediate();
  expect(container.querySelector("[data-block-anchor]")).toBe(surface);
});

it("does not animate historical tools on initial render", () => {
  const call = tool("historical-tool");
  const row = {
    message: message("historical-message", "root-run", call.id),
    facts: {
      toolCalls: { [call.id]: call },
      delegatedRuns: {},
      awaiting: new Map<string, string>(),
    },
  };
  const { container } = render(
    <MessageContext.Provider value={{ sessionId: "session-1", message: row.message }}>
      <AnimatePresence initial={false}>{renderMessageBlocks(row, CTX)}</AnimatePresence>
    </MessageContext.Provider>,
  );
  expect(container.querySelector<HTMLElement>("[data-block-anchor]")!.style.top).toBe("0px");
});

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
      awaiting: new Map<string, string>(),
    };

    renderRootTool(parentTool.id, facts);
    expect(screen.getAllByRole("button", { name: /Sub-agent.*Finished/ })).toHaveLength(1);
    expect(screen.queryByText("No narrative material yet")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: /Sub-agent.*Finished/ }));
    expect(navigator().get()).toMatchObject({ dock: "subagents", subagent: "child-run" });
    expect(screen.getAllByRole("button", { name: /Sub-agent.*Finished/ })).toHaveLength(1);

    const taskAnchors = document.querySelectorAll("#task-root, #task-child");
    expect(taskAnchors).toHaveLength(1);
  });

  it("names the delegation group once so its rows need not repeat the task", () => {
    const parentTool = tool("task-root");
    const facts: TurnFacts = {
      toolCalls: { [parentTool.id]: parentTool },
      delegatedRuns: {
        [parentTool.id]: [
          { run: run("child-a", "root-run", "root-run", parentTool.id), messages: [] },
          { run: run("child-b", "root-run", "root-run", parentTool.id), messages: [] },
        ],
      },
      awaiting: new Map<string, string>(),
    };

    renderRootTool(parentTool.id, facts);
    const group = screen.getByRole("group", { name: "Delegate work" });
    expect(within(group).getAllByRole("button", { name: /Sub-agent \d of 2/ })).toHaveLength(2);
    expect(within(group).queryByRole("button", { name: /Delegate work.*Sub-agent/ })).toBeNull();
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
      awaiting: new Map<string, string>(),
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
      facts: {
        toolCalls: { [call.id]: call },
        delegatedRuns: {},
        awaiting: new Map<string, string>(),
      },
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

it("only relocates the exact question already rendered by the composer", () => {
  const first = {
    kind: "question" as const,
    status: "complete" as const,
    itemId: "first",
    questions: [],
  };
  const second = { ...first, itemId: "second" };
  const row = {
    message: { ...message("questions", "root-run", "ask"), blocks: [first, second] },
    facts: { toolCalls: {}, delegatedRuns: {}, awaiting: new Map<string, string>() },
  };
  expect(renderMessageBlocks(row, CTX)).toHaveLength(2);
  expect(renderMessageBlocks(row, { ...CTX, questionInComposer: first })).toHaveLength(1);
  expect(renderMessageBlocks(row, { ...CTX, questionInComposer: { ...first } })).toHaveLength(2);
});

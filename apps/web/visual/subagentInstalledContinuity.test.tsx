import { act, fireEvent, render, renderHook, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useState, type ComponentProps } from "react";
import {
  type CapturedToolResult,
  type ComponentContext,
  type PluginComponent,
} from "@flame/component-sdk";
import { type FlameClient } from "@flame/runtime-client";
import { type TranscriptRow } from "@/features/agent/public/conversation";
import { RuntimeConnectionGeneration } from "@/features/connection/public/services";
import { InstalledToolResultComponent } from "@/features/plugins/ui/InstalledComponent";
import { definePlugin } from "@/extensions/definePlugin";
import { TOOL_RESULT_RENDERER } from "@/features/chat/public/tools/types";
import { loadPluginsForTest, resetKernelForTest } from "@/test/kernel";
import { type AgentRunView } from "@/features/agent/public/sessionView";
import { navigator } from "@/lib/navigation";
import {
  openWorkspaceSubagentRun,
  returnFromWorkspaceInspection,
  useWorkspaceInspection,
} from "@/features/workspace/public/navigation";
import { SubagentsPanel } from "@/features/chat/message/ui/SubagentsPanel";
const material = vi.hoisted(() => ({
  rows: [] as readonly TranscriptRow[],
  available: true,
  cancel: vi.fn(),
}));

afterEach(() => vi.restoreAllMocks());

vi.mock("@/features/agent/public/run", () => ({ cancelSessionRun: material.cancel }));

vi.mock("@/features/agent/public/conversation", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agent/public/conversation")>()),
  useActiveConversationRows: () => material.rows,
}));

vi.mock("@/features/connection/public/serviceStatus", () => ({
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
        awaiting: new Map<string, string>(),
      },
    },
  ];
}

it("keeps an installed result in the retained parent reader alive until the reader closes", async () => {
  material.rows = rows();
  const facts = material.rows[0]!.facts;
  const grep = {
    id: "grep",
    runId: "child",
    name: "grep",
    fn: "grep",
    args: "{}",
    status: "ok",
    result: "{}",
  } as const;
  facts.toolCalls.grep = grep;
  facts.toolCalls.nested = {
    id: "nested",
    runId: "child",
    name: "delegate_task",
    fn: "Review request headers",
    args: '{"summary":"Review request headers"}',
    status: "ok",
  };
  facts.delegatedRuns.delegate![0]!.messages[0]!.blocks.push(
    { kind: "tool", toolCallId: "grep" },
    { kind: "tool", toolCallId: "nested" },
  );
  const captured: CapturedToolResult = Object.freeze({ ...grep });
  const contexts: ComponentContext[] = [];
  function Result({ context }: ComponentProps<PluginComponent>) {
    if (contexts.at(-1) !== context) contexts.push(context);
    const [filter, setFilter] = useState("");
    return (
      <input
        aria-label="Filter result"
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
      />
    );
  }
  const resources = new AbortController();
  const component = { installationId: "grep", digest: "a".repeat(64), componentId: "grep" };
  const generation = RuntimeConnectionGeneration.forProcess("subagent-result-test");
  const compose = () => null;
  const runtime = {} as FlameClient;
  await loadPluginsForTest(
    definePlugin({
      name: "test.installed-result",
      setup(ctx) {
        const lifetime = ctx.lifetime("installed-result");
        ctx.contribute(
          TOOL_RESULT_RENDERER,
          {
            id: "installed-grep",
            title: "Installed grep",
            selector: { type: "toolName", name: "grep" },
            component: () => (
              <InstalledToolResultComponent
                entry={Result}
                component={component}
                generation={generation}
                compose={compose}
                runtime={runtime}
                binding={undefined}
                lifetime={lifetime}
                resources={resources.signal}
                toolResult={captured}
              />
            ),
          },
          { key: "installed-grep" },
        );
      },
    }),
  );
  try {
    navigator().go({ session: "session" });
    openWorkspaceSubagentRun("child");
    const { container } = render(
      <div data-dock-view-id="subagents">
        <SubagentsPanel />
      </div>,
    );
    const inspection = renderHook(useWorkspaceInspection);
    await act(async () => vi.dynamicImportSettled());
    fireEvent.click(container.querySelector('[data-tool="grep"] button')!);
    const filter = screen.getByRole<HTMLInputElement>("textbox", { name: "Filter result" });
    fireEvent.change(filter, { target: { value: "needle" } });
    const context = contexts.at(-1)!;
    const parent = screen.getByRole("region", { name: "Sub-agent" });

    fireEvent.click(within(parent).getByRole("button", { name: /Sub-agent/ }));
    expect(navigator().get().subagent).toBe("nested");
    expect(screen.queryByRole("textbox", { name: "Filter result" })).toBeNull();
    expect(context.signal.aborted).toBe(false);
    expect(filter.isConnected).toBe(true);

    act(() => returnFromWorkspaceInspection(inspection.result.current!));
    expect(screen.getByRole("textbox", { name: "Filter result" })).toBe(filter);
    expect(filter.value).toBe("needle");
    expect(contexts.at(-1)).toBe(context);
    expect(context.signal.aborted).toBe(false);

    fireEvent.click(screen.getByRole("button", { name: "All subagents" }));
    expect(filter.isConnected).toBe(false);
    expect(context.signal.aborted).toBe(true);
  } finally {
    resources.abort();
    await resetKernelForTest();
  }
});

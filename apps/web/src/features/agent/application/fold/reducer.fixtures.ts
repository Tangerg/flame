import {
  type AgentEventEnvelope as RunEvent,
  type AgentInterrupt,
  type AgentRunFact as RunRef,
  type AgentStreamEvent as StreamEvent,
} from "@/features/agent/public/events";
import {
  type AgentRunMetrics as RunMetrics,
  type AgentRunOutcome,
  type AgentSessionView,
  type ToolCategory,
} from "@/features/agent/public/sessionView";
import { selectCurrentRootRun } from "@/features/agent/application/view/runTree";
import { reduceAgentEvent } from "@/features/agent/application/fold/reducer";

// The runtime presenter stamps a category on every tool it projects; fixtures
// reproduce that wire so the fold reads the category it will be given.
const WIRE_TOOL_CATEGORY = new Map<string, ToolCategory>([
  ["shell", "shell"],
  ["read_shell_output", "shell"],
  ["stop_shell", "shell"],
  ["apply_patch", "edit"],
  ["edit", "edit"],
  ["read", "read"],
  ["read_skill_resource", "read"],
  ["read_tool_result", "read"],
  ["glob", "search"],
  ["grep", "search"],
  ["search_tools", "search"],
  ["web_search", "web"],
  ["web_fetch", "web"],
  ["ask_user", "task"],
  ["delegate_task", "task"],
  ["create_goal", "task"],
  ["get_goal", "task"],
  ["report_goal_outcome", "task"],
  ["load_skill", "task"],
  ["list_skills", "task"],
  ["propose_skill", "task"],
  ["enter_plan_mode", "task"],
  ["exit_plan_mode", "task"],
  ["set_plan", "task"],
]);

export function toolCategoryForName(name: string) {
  return WIRE_TOOL_CATEGORY.get(name) ?? "unknown";
}

export function stampDurableToolCategory<I extends { type: string }>(item: I): I {
  const candidate = item as unknown as {
    type: string;
    tool?: { name: string; category?: string };
  };
  if (
    candidate.type !== "toolCall" ||
    candidate.tool === undefined ||
    candidate.tool.category !== undefined
  ) {
    return item;
  }
  return {
    ...item,
    tool: { ...candidate.tool, category: toolCategoryForName(candidate.tool.name) },
  } as I;
}

function stampToolCategory(event: StreamEvent): StreamEvent {
  if (
    (event.type === "item.started" || event.type === "item.completed") &&
    event.item.type === "toolCall" &&
    event.item.tool.category === undefined
  ) {
    return { ...event, item: stampDurableToolCategory(event.item) };
  }
  if (event.type === "segment.finished") {
    return {
      ...event,
      interrupts: event.interrupts.map((interrupt) =>
        interrupt.type === "approval" && interrupt.payload.tool.category === undefined
          ? {
              ...interrupt,
              payload: {
                ...interrupt.payload,
                tool: {
                  ...interrupt.payload.tool,
                  category: toolCategoryForName(interrupt.payload.tool.name),
                },
              },
            }
          : interrupt,
      ),
    };
  }
  return event;
}
export const noMetrics: RunMetrics = {
  steps: 0,
  activeDurationMillis: 0,
};

export const FINISHED_AT = "2026-06-03T00:05:00.000Z";

let nextEventSequence = 0;

function completeStartedRun(state: AgentSessionView, run: RunRef, segmentId: string): RunRef {
  const parent = run.spawnedByItemId ? selectCurrentRootRun(state) : null;
  const { createdAt = "2026-06-03T00:00:00.000Z", metrics = noMetrics, status = "running" } = run;
  const child = run.spawnedByItemId !== null && run.spawnedByItemId !== undefined;
  return {
    id: run.id,
    sessionId: run.sessionId,
    parentRunId: child ? (run.parentRunId ?? parent?.id ?? null) : null,
    rootRunId: child ? (run.rootRunId ?? parent?.rootRunId ?? parent?.id ?? run.id) : run.id,
    spawnedByItemId: run.spawnedByItemId ?? null,
    activeSegmentId: segmentId,
    createdAt,
    metrics,
    status,
    outcome: null,
    finishedAt: null,
  };
}

function completeFinishedRun(state: AgentSessionView, run: RunRef, runId: string): RunRef {
  const owner = state.runsById[runId];
  return {
    ...run,
    id: runId,
    sessionId: owner?.sessionId ?? "ses_1",
    parentRunId: owner?.parentRunId ?? null,
    rootRunId: owner?.rootRunId ?? runId,
    spawnedByItemId: owner?.spawnedByItemId ?? null,
    activeSegmentId: null,
    createdAt: owner?.createdAt ?? "2026-06-03T00:00:00.000Z",
    ...(owner?.modelSelection ? { modelSelection: { ...owner.modelSelection } } : {}),
  };
}

export function testRunEvent(
  state: AgentSessionView,
  event: StreamEvent,
  runId?: string,
  segmentId?: string,
): RunEvent {
  event = stampToolCategory(event);
  const payloadRunId =
    event.type === "segment.started"
      ? event.run.id
      : event.type === "item.started" || event.type === "item.completed"
        ? event.item.runId
        : undefined;
  const ownerRunId = runId ?? payloadRunId ?? selectCurrentRootRun(state)?.id ?? "run_1";
  const owner = state.runsById[ownerRunId];
  const ownerSegmentId =
    segmentId ??
    (event.type === "segment.started" ? event.run.activeSegmentId : owner?.activeSegmentId) ??
    `seg_${ownerRunId}`;
  const sequence = ++nextEventSequence;
  const normalizedEvent: StreamEvent =
    event.type === "segment.started"
      ? { ...event, run: completeStartedRun(state, event.run, ownerSegmentId) }
      : event.type === "segment.finished"
        ? { ...event, run: completeFinishedRun(state, event.run, ownerRunId) }
        : event;
  return {
    event: normalizedEvent,
    eventId: `evt_test_${sequence}`,
    runId: ownerRunId,
    segmentId: ownerSegmentId,
    timestamp: `2026-06-03T00:00:${String(sequence % 60).padStart(2, "0")}.000Z`,
  };
}

export function foldTestEvent(
  state: AgentSessionView,
  event: StreamEvent,
  runId?: string,
  segmentId?: string,
): AgentSessionView {
  return reduceAgentEvent(state, testRunEvent(state, event, runId, segmentId));
}

export const runFinished = (
  outcome: AgentRunOutcome,
  metrics: RunMetrics = noMetrics,
  contextTokens?: number,
): StreamEvent => ({
  type: "segment.finished",
  run: {
    status: "finished",
    outcome,
    metrics,
    ...(contextTokens !== undefined ? { contextTokens } : {}),
    finishedAt: FINISHED_AT,
  } as RunRef,
  interrupts: [],
});

export const runWaiting = (
  interrupts: AgentInterrupt[] = [],
  metrics: RunMetrics = noMetrics,
  contextTokens?: number,
): StreamEvent => ({
  type: "segment.finished",
  run: {
    status: "waiting",
    outcome: null,
    metrics,
    ...(contextTokens !== undefined ? { contextTokens } : {}),
    finishedAt: null,
  } as RunRef,
  interrupts,
});

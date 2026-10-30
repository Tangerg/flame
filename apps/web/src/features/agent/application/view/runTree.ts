import {
  type AgentProblem,
  type AgentRunView,
  type AgentSessionView,
  type Message,
} from "@/features/agent/public/sessionView";
import { isAgentRunFailure } from "@/features/agent/application/view/runOutcome";
const EMPTY_MESSAGES: Message[] = [];

export interface DelegatedRunNarrative {
  run: AgentRunView;
  messages: Message[];
}

export type DelegatedRunNarrativesByItemId = Record<string, DelegatedRunNarrative[]>;

type AgentRootAttention =
  { status: "idle"; runId: null } | { status: AgentRunView["status"]; runId: string };

function compareRuns(left: AgentRunView, right: AgentRunView): number {
  const byCreatedAt = left.createdAt.localeCompare(right.createdAt);
  return byCreatedAt || left.id.localeCompare(right.id);
}

function selectRootRuns(view: AgentSessionView): AgentRunView[] {
  return Object.values(view.runsById)
    .filter((run) => run.parentRunId === null)
    .sort(compareRuns);
}

export function selectRootRollbackBoundary(
  view: AgentSessionView,
  runId: string,
): { toRunId?: string } | null {
  const roots = selectRootRuns(view);
  const index = roots.findIndex((run) => run.id === runId);
  if (index < 0) return null;
  return index > 0 ? { toRunId: roots[index - 1]!.id } : {};
}

export function selectCurrentRootRun(view: AgentSessionView): AgentRunView | null {
  let latest: AgentRunView | null = null;
  let latestOpen: AgentRunView | null = null;
  for (const run of Object.values(view.runsById)) {
    if (run.parentRunId !== null) continue;
    if (!latest || compareRuns(latest, run) < 0) latest = run;
    if (run.status !== "finished" && (!latestOpen || compareRuns(latestOpen, run) < 0)) {
      latestOpen = run;
    }
  }
  return latestOpen ?? latest;
}

export function selectCurrentRootAttention(view: AgentSessionView): AgentRootAttention {
  const root = selectCurrentRootRun(view);
  return root ? { status: root.status, runId: root.id } : { status: "idle", runId: null };
}

export function selectRootNarrativeMessages(view: AgentSessionView): Message[] {
  const rootRunIds = new Set(selectRootRuns(view).map((run) => run.id));
  const messages = view.messages.filter(
    (message) => message.runId === null || rootRunIds.has(message.runId),
  );
  return messages.length > 0 ? messages : EMPTY_MESSAGES;
}

export function selectDelegatedRunNarratives(
  view: AgentSessionView,
): DelegatedRunNarrativesByItemId {
  const messagesByRunId = new Map<string, Message[]>();
  for (const message of view.messages) {
    if (message.runId === null) continue;
    const bucket = messagesByRunId.get(message.runId);
    if (bucket === undefined) messagesByRunId.set(message.runId, [message]);
    else bucket.push(message);
  }

  const byItemId: DelegatedRunNarrativesByItemId = {};
  for (const run of Object.values(view.runsById).sort(compareRuns)) {
    if (run.parentRunId === null || run.spawnedByItemId === null) continue;
    const narrative: DelegatedRunNarrative = {
      run,
      messages: messagesByRunId.get(run.id) ?? EMPTY_MESSAGES,
    };
    (byItemId[run.spawnedByItemId] ??= []).push(narrative);
  }
  return byItemId;
}

export function selectRunProblem(run: AgentRunView | null): AgentProblem | null {
  return isAgentRunFailure(run?.outcome) ? run.outcome.error : null;
}

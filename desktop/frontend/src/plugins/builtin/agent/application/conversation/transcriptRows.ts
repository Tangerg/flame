import type {
  AgentRunStatus,
  AgentSessionView,
  Message,
  ToolCall,
} from "@/plugins/sdk/types/agentSessionView";
import type { DelegatedRunNarrativesByItemId } from "../view/runTree";
import { selectDelegatedRunNarratives, selectRootNarrativeMessages } from "../view/runTree";
import { selectAwaitingInterrupts } from "../view/awaitingInterrupts";

export interface TurnFacts {
  toolCalls: Record<string, ToolCall>;
  delegatedRuns: DelegatedRunNarrativesByItemId;
  /** The Run to resume for each of this turn's Items that awaits an answer. */
  awaiting: ReadonlyMap<string, string>;
}

type TranscriptRunOwner =
  { kind: "unassigned" } | { kind: "owned"; runId: string; status: AgentRunStatus };

export interface TranscriptRow {
  message: Message;
  runOwner: TranscriptRunOwner;
  facts: TurnFacts;
}

const NO_TOOL_CALLS: Record<string, ToolCall> = {};
const NO_DELEGATED_RUNS: DelegatedRunNarrativesByItemId = {};
const NO_AWAITING: ReadonlyMap<string, string> = new Map();
const NO_FACTS: TurnFacts = {
  toolCalls: NO_TOOL_CALLS,
  delegatedRuns: NO_DELEGATED_RUNS,
  awaiting: NO_AWAITING,
};
const NO_ROWS: readonly TranscriptRow[] = [];

interface CachedRow {
  row: TranscriptRow;
  identities: readonly unknown[];
}

export type TranscriptRowCache = ReadonlyMap<string, CachedRow>;

export const EMPTY_TRANSCRIPT_ROW_CACHE: TranscriptRowCache = new Map();

interface TranscriptRowBuild {
  rows: readonly TranscriptRow[];
  cache: TranscriptRowCache;
}

export function buildTranscriptRows(
  view: AgentSessionView,
  previous: TranscriptRowCache,
): TranscriptRowBuild {
  const messages = selectRootNarrativeMessages(view);
  if (messages.length === 0) return { rows: NO_ROWS, cache: EMPTY_TRANSCRIPT_ROW_CACHE };

  const delegated = selectDelegatedRunNarratives(view);
  const awaiting = selectAwaitingInterrupts(view);
  const rows: TranscriptRow[] = [];
  const cache = new Map<string, CachedRow>();

  for (const message of messages) {
    const runOwner = transcriptRunOwner(message, view);
    const { facts, identities } = readTurnFacts(message, view.toolCalls, delegated, awaiting);
    const rowIdentities = [runOwnerIdentity(runOwner), ...identities];
    const cached = previous.get(message.id);
    if (cached !== undefined && sameIdentities(cached.identities, rowIdentities)) {
      rows.push(cached.row);
      cache.set(message.id, cached);
      continue;
    }
    const entry: CachedRow = { row: { message, runOwner, facts }, identities: rowIdentities };
    rows.push(entry.row);
    cache.set(message.id, entry);
  }

  return { rows, cache };
}

function transcriptRunOwner(message: Message, view: AgentSessionView): TranscriptRunOwner {
  if (message.runId === null) return { kind: "unassigned" };
  const run = view.runsById[message.runId];
  if (!run) {
    throw new Error(`agent.transcript.runMissing:message=${message.id};run=${message.runId}`);
  }
  return { kind: "owned", runId: run.id, status: run.status };
}

function runOwnerIdentity(owner: TranscriptRunOwner): string {
  return owner.kind === "owned" ? `${owner.kind}:${owner.status}` : owner.kind;
}

function readTurnFacts(
  message: Message,
  sessionToolCalls: Record<string, ToolCall>,
  delegated: DelegatedRunNarrativesByItemId,
  sessionAwaiting: ReadonlyMap<string, string>,
): { facts: TurnFacts; identities: readonly unknown[] } {
  const identities: unknown[] = [message];
  let toolCalls: Record<string, ToolCall> | undefined;
  let delegatedRuns: DelegatedRunNarrativesByItemId | undefined;
  let awaiting: Map<string, string> | undefined;

  for (const block of message.blocks) {
    const itemId = awaitableItemId(block);
    const resumeRunId = itemId === undefined ? undefined : sessionAwaiting.get(itemId);
    if (itemId === undefined || resumeRunId === undefined) continue;
    awaiting ??= new Map();
    awaiting.set(itemId, resumeRunId);
    identities.push(`awaiting:${itemId}:${resumeRunId}`);
  }

  const pending: Message[] = [message];
  const visitedRuns = new Set<string>();

  for (let cursor = 0; cursor < pending.length; cursor += 1) {
    for (const block of pending[cursor]!.blocks) {
      if (block.kind !== "tool") continue;
      const toolCallId = block.toolCallId;

      const call = sessionToolCalls[toolCallId];
      if (call !== undefined) {
        toolCalls ??= {};
        if (toolCalls[toolCallId] === undefined) {
          toolCalls[toolCallId] = call;
          identities.push(call);
        }
      }

      const narratives = delegated[toolCallId];
      if (narratives === undefined) continue;
      delegatedRuns ??= {};
      delegatedRuns[toolCallId] = narratives;
      for (const narrative of narratives) {
        if (visitedRuns.has(narrative.run.id)) continue;
        visitedRuns.add(narrative.run.id);
        identities.push(narrative.run);
        for (const nested of narrative.messages) {
          identities.push(nested);
          pending.push(nested);
        }
      }
    }
  }

  const facts =
    toolCalls === undefined && delegatedRuns === undefined && awaiting === undefined
      ? NO_FACTS
      : {
          toolCalls: toolCalls ?? NO_TOOL_CALLS,
          delegatedRuns: delegatedRuns ?? NO_DELEGATED_RUNS,
          awaiting: awaiting ?? NO_AWAITING,
        };
  return { facts, identities };
}

function awaitableItemId(block: Message["blocks"][number]): string | undefined {
  switch (block.kind) {
    case "approval":
    case "question":
      return block.itemId;
    case "tool":
      return block.toolCallId;
    default:
      return undefined;
  }
}

function sameIdentities(left: readonly unknown[], right: readonly unknown[]): boolean {
  if (left.length !== right.length) return false;
  for (let index = 0; index < left.length; index += 1) {
    if (left[index] !== right[index]) return false;
  }
  return true;
}

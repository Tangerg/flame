import type {
  AgentRunStatus,
  AgentSessionView,
  Message,
  ToolCall,
} from "@/plugins/sdk/types/agentSessionView";
import type { DelegatedRunNarrativesByItemId } from "../view/runTree";
import { selectDelegatedRunNarratives, selectRootNarrativeMessages } from "../view/runTree";

export interface TurnFacts {
  toolCalls: Record<string, ToolCall>;
  delegatedRuns: DelegatedRunNarrativesByItemId;
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
const NO_FACTS: TurnFacts = { toolCalls: NO_TOOL_CALLS, delegatedRuns: NO_DELEGATED_RUNS };
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
  const rows: TranscriptRow[] = [];
  const cache = new Map<string, CachedRow>();

  for (const message of messages) {
    const runOwner = transcriptRunOwner(message, view);
    const { facts, identities } = readTurnFacts(message, view.toolCalls, delegated);
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
): { facts: TurnFacts; identities: readonly unknown[] } {
  const identities: unknown[] = [message];
  let toolCalls: Record<string, ToolCall> | undefined;
  let delegatedRuns: DelegatedRunNarrativesByItemId | undefined;

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
    toolCalls === undefined && delegatedRuns === undefined
      ? NO_FACTS
      : {
          toolCalls: toolCalls ?? NO_TOOL_CALLS,
          delegatedRuns: delegatedRuns ?? NO_DELEGATED_RUNS,
        };
  return { facts, identities };
}

function sameIdentities(left: readonly unknown[], right: readonly unknown[]): boolean {
  if (left.length !== right.length) return false;
  for (let index = 0; index < left.length; index += 1) {
    if (left[index] !== right[index]) return false;
  }
  return true;
}

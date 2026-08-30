import type {
  AgentRunStatus,
  AgentSessionView,
  Message,
  ToolCall,
} from "@/plugins/sdk/types/agentSessionView";
import type { DelegatedRunNarrativesByItemId } from "../view/runTree";
import { selectDelegatedRunNarratives, selectRootNarrativeMessages } from "../view/runTree";

/**
 * The session facts one turn renders from, NARROWED to that turn — a turn showing no tool
 * call holds an empty map, so a tool streaming arguments elsewhere in the session cannot
 * invalidate it. Reaches through delegation transitively.
 */
export interface TurnFacts {
  toolCalls: Record<string, ToolCall>;
  delegatedRuns: DelegatedRunNarrativesByItemId;
}

// `unassigned` is only ever an optimistic turn that has not received its Run id: the root
// narrative selector already excludes material whose Run is absent.
export type TranscriptRunOwner =
  { kind: "unassigned" } | { kind: "owned"; runId: string; status: AgentRunStatus };

export interface TranscriptRow {
  message: Message;
  runOwner: TranscriptRunOwner;
  facts: TurnFacts;
}

// SHARED empties: a row survives a rebuild only when everything it holds is identical, so
// per-turn `{}` would make every text-only row a fresh object on every delta.
const NO_TOOL_CALLS: Record<string, ToolCall> = {};
const NO_DELEGATED_RUNS: DelegatedRunNarrativesByItemId = {};
const NO_FACTS: TurnFacts = { toolCalls: NO_TOOL_CALLS, delegatedRuns: NO_DELEGATED_RUNS };
const NO_ROWS: readonly TranscriptRow[] = [];

interface CachedRow {
  row: TranscriptRow;
  identities: readonly unknown[];
}

/** Opaque to callers: hand back whatever the previous build returned. Starting from
 *  `EMPTY_TRANSCRIPT_ROW_CACHE` is always correct — it costs one full rebuild. */
export type TranscriptRowCache = ReadonlyMap<string, CachedRow>;

export const EMPTY_TRANSCRIPT_ROW_CACHE: TranscriptRowCache = new Map();

interface TranscriptRowBuild {
  rows: readonly TranscriptRow[];
  cache: TranscriptRowCache;
}

/**
 * Pure: same `(view, previous)` gives the same result and `previous` is only ever read.
 * Feeding the returned cache back is what makes row reuse compound across a stream.
 */
export function buildTranscriptRows(
  view: AgentSessionView,
  previous: TranscriptRowCache,
): TranscriptRowBuild {
  const messages = selectRootNarrativeMessages(view);
  if (messages.length === 0) return { rows: NO_ROWS, cache: EMPTY_TRANSCRIPT_ROW_CACHE };

  const delegated = selectDelegatedRunNarratives(view);
  const rows: TranscriptRow[] = [];
  // Rebuilt rather than mutated, so a turn that left the transcript leaves the cache
  // with it instead of pinning its messages alive for the rest of the session.
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

// Facts and identities come out of ONE walk on purpose: they are the same question asked
// twice, and deriving them separately lets the invalidation rule drift from what is
// rendered — which shows up as a turn that stops updating.
function readTurnFacts(
  message: Message,
  sessionToolCalls: Record<string, ToolCall>,
  delegated: DelegatedRunNarrativesByItemId,
): { facts: TurnFacts; identities: readonly unknown[] } {
  const identities: unknown[] = [message];
  let toolCalls: Record<string, ToolCall> | undefined;
  let delegatedRuns: DelegatedRunNarrativesByItemId | undefined;

  // Cursor rather than shift() so the queue is not re-indexed per step. `visitedRuns`
  // bounds the walk: a malformed lineage pointing a subagent back at an ancestor's item
  // would otherwise never terminate.
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
        // The wrapper is rebuilt on every projection, so its own identity says nothing;
        // what it HOLDS comes from the view and is stable until the fold replaces it.
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

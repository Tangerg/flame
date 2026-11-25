import {
  type AgentRunStatus,
  type AgentSessionView,
  type Message,
  type ToolCall,
} from "@/features/agent/public/sessionView";
import {
  type DelegatedRunNarrative,
  type DelegatedRunNarrativesByItemId,
} from "@/features/agent/application/view/runTree";
import {
  selectDelegatedRunNarratives,
  selectRootNarrativeMessages,
} from "@/features/agent/application/view/runTree";
import { selectAwaitingInterrupts } from "@/features/agent/application/view/awaitingInterrupts";
import {
  steerInputFeedback,
  type SteerInputFeedback,
} from "@/features/agent/application/view/steerInputFeedback";
export interface TurnFacts {
  toolCalls: Record<string, ToolCall>;
  delegatedRuns: DelegatedRunNarrativesByItemId;
  awaiting: ReadonlyMap<string, string>;
}

type TranscriptRunOwner =
  { kind: "unassigned" } | { kind: "owned"; runId: string; status: AgentRunStatus };

export interface TranscriptRow {
  inputFeedback?: SteerInputFeedback;
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
    const inputFeedback = steerInputFeedback(message);
    const runOwner = transcriptRunOwner(message, view);
    const { facts, identities } = readTurnFacts(message, view.toolCalls, delegated, awaiting);
    const rowIdentities = [runOwnerIdentity(runOwner), inputFeedback, ...identities];
    const cached = previous.get(message.id);
    if (cached !== undefined && sameIdentities(cached.identities, rowIdentities)) {
      rows.push(cached.row);
      cache.set(message.id, cached);
      continue;
    }
    const entry: CachedRow = {
      row: { message, runOwner, facts, ...(inputFeedback ? { inputFeedback } : {}) },
      identities: rowIdentities,
    };
    rows.push(entry.row);
    cache.set(message.id, entry);
  }

  return { rows, cache };
}

export function delegatedTranscriptRows(
  { run, messages }: DelegatedRunNarrative,
  facts: TurnFacts,
): TranscriptRow[] {
  return messages.map((message) => ({
    message,
    runOwner: { kind: "owned", runId: run.id, status: run.status },
    inputFeedback: steerInputFeedback(message),
    facts: readTurnFacts(message, facts.toolCalls, facts.delegatedRuns, facts.awaiting).facts,
  }));
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

// Collects what a turn's blocks reference, following delegated runs into their
// nested messages once each; identities record every input the facts read.
class TurnFactCollector {
  readonly identities: unknown[];
  readonly pending: Message[];
  #toolCalls: Record<string, ToolCall> | undefined;
  #delegatedRuns: DelegatedRunNarrativesByItemId | undefined;
  #awaiting: Map<string, string> | undefined;
  readonly #visitedRuns = new Set<string>();
  readonly #sessionToolCalls: Record<string, ToolCall>;
  readonly #delegated: DelegatedRunNarrativesByItemId;
  readonly #sessionAwaiting: ReadonlyMap<string, string>;

  constructor(
    message: Message,
    sessionToolCalls: Record<string, ToolCall>,
    delegated: DelegatedRunNarrativesByItemId,
    sessionAwaiting: ReadonlyMap<string, string>,
  ) {
    this.identities = [message];
    this.pending = [message];
    this.#sessionToolCalls = sessionToolCalls;
    this.#delegated = delegated;
    this.#sessionAwaiting = sessionAwaiting;
  }

  visit(block: Message["blocks"][number]): void {
    this.#awaitable(block);
    if (block.kind === "tool") {
      this.#toolCall(block.itemId);
      this.#delegatedNarratives(block.itemId);
    }
  }

  facts(): TurnFacts {
    if (
      this.#toolCalls === undefined &&
      this.#delegatedRuns === undefined &&
      this.#awaiting === undefined
    )
      return NO_FACTS;
    return {
      toolCalls: this.#toolCalls ?? NO_TOOL_CALLS,
      delegatedRuns: this.#delegatedRuns ?? NO_DELEGATED_RUNS,
      awaiting: this.#awaiting ?? NO_AWAITING,
    };
  }

  #awaitable(block: Message["blocks"][number]): void {
    const itemId = awaitableItemId(block);
    if (itemId === undefined || this.#awaiting?.has(itemId)) return;
    const resumeRunId = this.#sessionAwaiting.get(itemId);
    if (resumeRunId === undefined) return;
    this.#awaiting ??= new Map();
    this.#awaiting.set(itemId, resumeRunId);
    this.identities.push(`awaiting:${itemId}:${resumeRunId}`);
  }

  #toolCall(toolCallId: string): void {
    const call = this.#sessionToolCalls[toolCallId];
    if (call === undefined) return;
    this.#toolCalls ??= {};
    if (this.#toolCalls[toolCallId] !== undefined) return;
    this.#toolCalls[toolCallId] = call;
    this.identities.push(call);
  }

  #delegatedNarratives(toolCallId: string): void {
    const narratives = this.#delegated[toolCallId];
    if (narratives === undefined) return;
    this.#delegatedRuns ??= {};
    this.#delegatedRuns[toolCallId] = narratives;
    for (const narrative of narratives) {
      if (this.#visitedRuns.has(narrative.run.id)) continue;
      this.#visitedRuns.add(narrative.run.id);
      this.identities.push(narrative.run);
      for (const nested of narrative.messages) {
        this.identities.push(nested);
        this.pending.push(nested);
      }
    }
  }
}

function readTurnFacts(
  message: Message,
  sessionToolCalls: Record<string, ToolCall>,
  delegated: DelegatedRunNarrativesByItemId,
  sessionAwaiting: ReadonlyMap<string, string>,
): { facts: TurnFacts; identities: readonly unknown[] } {
  const collector = new TurnFactCollector(message, sessionToolCalls, delegated, sessionAwaiting);
  for (let cursor = 0; cursor < collector.pending.length; cursor += 1) {
    for (const block of collector.pending[cursor]!.blocks) collector.visit(block);
  }
  return { facts: collector.facts(), identities: collector.identities };
}

function awaitableItemId(block: Message["blocks"][number]): string | undefined {
  switch (block.kind) {
    case "approval":
    case "question":
      return block.itemId;
    case "tool":
      return block.itemId;
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

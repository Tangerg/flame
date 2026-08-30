import type { ContentBlock } from "@/plugins/sdk/types/contentBlock";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { isQuestionTool } from "../domain/toolCategory";
import { isReadOnlyTool } from "./toolPresentation";

// Carried on the unit rather than recomputed by the renderer: the planner already asks the
// same question to decide what to fold, and two places deriving one rule drift apart.
// A `wave` needs no flag — it exists only because an answer followed it.
type Superseded = { superseded: boolean };

export type MessageRenderUnit =
  | ({ kind: "block"; block: ContentBlock; index: number } & Superseded)
  | ({ kind: "toolGroup"; tools: ToolCall[] } & Superseded)
  | { kind: "wave"; units: MessageRenderUnit[] };

// Approvals and questions are NOT process even though they arrive mid-turn: they ask the
// reader for something, and folding away a request for a decision ends the turn in silence.
function isProcess(block: ContentBlock): boolean {
  return block.kind === "reasoning" || block.kind === "tool";
}

interface PositionedBlock {
  block: ContentBlock;
  index: number;
}

/**
 * Plans one message's blocks into the units the transcript renders.
 *
 * Every run of process blocks that already has prose after it folds into a single `wave`,
 * so a long turn reads as work · answer · work · answer rather than as everything the agent
 * ever did at one weight. The run still in flight is NEVER folded — that is the one the
 * reader is watching.
 */
export function planRenderUnits(
  blocks: ContentBlock[],
  toolCalls: Record<string, ToolCall>,
  answerFollows = false,
): MessageRenderUnit[] {
  const hasQuestion = blocks.some((block) => block.kind === "question");
  const approvalOwnedToolCallIds = findApprovalOwnedToolCallIds(blocks, toolCalls);
  const answered = answeredAfter(blocks, answerFollows);
  const units: MessageRenderUnit[] = [];
  let wave: PositionedBlock[] = [];

  const flushWave = () => {
    if (wave.length === 0) return;
    const inner = planWithinWave(wave, toolCalls, hasQuestion, approvalOwnedToolCallIds, answered);
    const last = wave[wave.length - 1]!;
    // Two units minimum: a run that already plans to one row folds on its own, and
    // wrapping it would only add a level to open through.
    if (answered[last.index] && inner.length >= 2) units.push({ kind: "wave", units: inner });
    else units.push(...inner);
    wave = [];
  };

  blocks.forEach((block, index) => {
    if (isProcess(block)) {
      wave.push({ block, index });
      return;
    }
    flushWave();
    units.push({ kind: "block", block, index, superseded: answered[index]! });
  });

  flushWave();
  return units;
}

// Walks BACKWARDS: the question is about what comes after each block. A block must carry
// TEXT to count — `item.started` creates the answer's block before a token exists, so
// treating its presence as the answer folds the thinking away the instant the model opens
// its reply, which for providers that stream reasoning and prose from overlapping items is
// the whole time it is thinking.
function answeredAfter(blocks: ContentBlock[], answerFollows: boolean): boolean[] {
  const answered: boolean[] = Array.from({ length: blocks.length }, () => false);
  let seen = answerFollows;
  for (let index = blocks.length - 1; index >= 0; index -= 1) {
    answered[index] = seen;
    const block = blocks[index]!;
    if (block.kind === "text" && block.text.trim() !== "") seen = true;
  }
  return answered;
}

function planWithinWave(
  positioned: readonly PositionedBlock[],
  toolCalls: Record<string, ToolCall>,
  hasQuestion: boolean,
  approvalOwnedToolCallIds: ReadonlySet<string>,
  answered: readonly boolean[],
): MessageRenderUnit[] {
  const units: MessageRenderUnit[] = [];
  let reads: PositionedBlock[] = [];

  const flushReads = () => {
    if (reads.length >= 2) {
      units.push({
        kind: "toolGroup",
        tools: reads.map((item) => toolOf(item.block, toolCalls)!),
        superseded: answered[reads[reads.length - 1]!.index]!,
      });
    } else {
      for (const item of reads) {
        units.push({
          kind: "block",
          block: item.block,
          index: item.index,
          superseded: answered[item.index]!,
        });
      }
    }
    reads = [];
  };

  for (const item of positioned) {
    const tool = toolOf(item.block, toolCalls);
    // The fold keeps both facts while a call waits — the ToolCall is the durable operation,
    // the approval block the actionable interruption — but the transcript exposes ONE
    // request surface. Matched only while both are pending, so the historical tool row
    // returns as soon as the decision settles.
    if (tool && approvalOwnedToolCallIds.has(tool.id)) {
      flushReads();
      continue;
    }
    // Checked AHEAD of grouping: a question's own tool is side-effect-free (it IS the
    // interrupt), so it reads as a glance and would be folded into a group instead of
    // dropped in favour of the question card.
    if (tool && hasQuestion && isQuestionTool(tool.name)) {
      flushReads();
      continue;
    }
    if (tool && isReadOnlyTool(tool)) {
      reads.push(item);
      continue;
    }
    flushReads();
    units.push({
      kind: "block",
      block: item.block,
      index: item.index,
      superseded: answered[item.index]!,
    });
  }

  flushReads();
  return units;
}

function findApprovalOwnedToolCallIds(
  blocks: readonly ContentBlock[],
  toolCalls: Record<string, ToolCall>,
): ReadonlySet<string> {
  const ids = new Set<string>();
  for (const block of blocks) {
    if (block.kind !== "approval" || block.status !== "requires-action" || !block.itemId) {
      continue;
    }
    if (toolCalls[block.itemId]?.status === "requires-action") ids.add(block.itemId);
  }
  return ids;
}

function toolOf(block: ContentBlock, toolCalls: Record<string, ToolCall>): ToolCall | undefined {
  return block.kind === "tool" ? toolCalls[block.toolCallId] : undefined;
}

// Counts reasoning as a step, not just tool calls: a wave of four commands and two
// conclusions holds six things, and a count of four under-reports what opening it reveals.
export function waveStepCount(units: readonly MessageRenderUnit[]): number {
  let steps = 0;
  for (const unit of units) {
    if (unit.kind === "toolGroup") steps += unit.tools.length;
    else if (unit.kind === "block" && unit.block.kind === "tool") steps += 1;
    else if (unit.kind === "block" && unit.block.kind === "reasoning") steps += 1;
  }
  return steps;
}

// Lives beside `waveStepCount` because the planner is the only thing that knows a wave's
// members without walking blocks a second time.
export function waveToolCalls(
  units: readonly MessageRenderUnit[],
  toolCalls: Record<string, ToolCall>,
): ToolCall[] {
  const tools: ToolCall[] = [];
  for (const unit of units) {
    if (unit.kind === "toolGroup") {
      tools.push(...unit.tools);
      continue;
    }
    if (unit.kind !== "block" || unit.block.kind !== "tool") continue;
    const tool = toolCalls[unit.block.toolCallId];
    if (tool) tools.push(tool);
  }
  return tools;
}

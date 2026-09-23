import type { ContentBlock } from "@/plugins/sdk/types/contentBlock";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { isQuestionTool } from "../domain/toolCategory";
import { isReadOnlyTool } from "./toolPresentation";

type Superseded = { superseded: boolean };

export type MessageRenderUnit =
  | ({ kind: "block"; block: ContentBlock; index: number } & Superseded)
  | ({ kind: "toolGroup"; tools: ToolCall[] } & Superseded)
  | { kind: "wave"; units: MessageRenderUnit[] };

function isProcess(block: ContentBlock): boolean {
  return block.kind === "reasoning" || block.kind === "tool";
}

interface PositionedBlock {
  block: ContentBlock;
  index: number;
}

const EMPTY_DELEGATING: ReadonlySet<string> = new Set();

export function planRenderUnits(
  blocks: ContentBlock[],
  toolCalls: Record<string, ToolCall>,
  answerFollows = false,
  delegating: ReadonlySet<string> = EMPTY_DELEGATING,
): MessageRenderUnit[] {
  const hasQuestion = blocks.some((block) => block.kind === "question");
  const approvalOwnedToolCallIds = findApprovalOwnedToolCallIds(blocks, toolCalls);
  const answered = answeredAfter(blocks, answerFollows);
  const units: MessageRenderUnit[] = [];
  let wave: PositionedBlock[] = [];

  const flushWave = () => {
    if (wave.length === 0) return;
    const inner = planWithinWave(
      wave,
      toolCalls,
      hasQuestion,
      approvalOwnedToolCallIds,
      answered,
      delegating,
    );
    const last = wave[wave.length - 1]!;
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
  delegating: ReadonlySet<string>,
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
    if (tool && approvalOwnedToolCallIds.has(tool.id)) {
      flushReads();
      continue;
    }
    if (
      tool &&
      hasQuestion &&
      isQuestionTool(tool.name) &&
      tool.status !== "err" &&
      tool.status !== "denied"
    ) {
      flushReads();
      continue;
    }
    if (tool && isReadOnlyTool(tool) && !delegating.has(tool.id)) {
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

export function waveStepCount(units: readonly MessageRenderUnit[]): number {
  let steps = 0;
  for (const unit of units) {
    if (unit.kind === "toolGroup") steps += unit.tools.length;
    else if (unit.kind === "block" && unit.block.kind === "tool") steps += 1;
    else if (unit.kind === "block" && unit.block.kind === "reasoning") steps += 1;
  }
  return steps;
}

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

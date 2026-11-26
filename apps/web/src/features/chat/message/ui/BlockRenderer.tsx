import * as stylex from "@stylexjs/stylex";
import { motion } from "motion/react";
import { space } from "@flame/ui/tokens.stylex";
import { stepEnter } from "@/lib/motion";
import { type ContentBlock } from "@/features/agent/public/contentBlock";
import { type TranscriptRow, type TurnFacts } from "@/features/agent/public/conversation";
import { type MessageRenderUnit } from "@/features/agent/public/messagePresentation";
import { type BlockCtx } from "@/features/chat/message/ui/blockContext";
import { MarkdownMessage } from "@/features/chat/message/ui/markdown/MarkdownMessage";
import { ApprovalCard } from "@/features/chat/message/ui/cards/index";
import { CompactionBlock } from "@/features/chat/message/ui/cards/index";
import { ImageBlock } from "@/features/chat/message/ui/cards/index";
import { QuestionCard } from "@/features/chat/message/ui/cards/index";
import { ReasoningBlock } from "@/features/chat/message/ui/cards/index";
import { ReferenceBlock } from "@/features/chat/message/ui/cards/index";
import {
  ToolCard,
  ToolGroup,
  type StandaloneToolResult,
  type StandaloneToolResultFor,
} from "@/features/chat/public/tools/rendering";
import { lookupExtensionByKey } from "@/extensions/selectors/extensions";
import {
  TOOL_STANDING_SURFACE,
  type ToolResultRendererSpec,
} from "@/features/chat/public/tools/types";
import {
  messageBlockRenderUnits,
  narratedBlocks,
} from "@/features/chat/message/application/messageBlockModel";
import {
  BLOCK_ANCHOR_ATTR,
  renderUnitAnchor,
  renderUnitKey,
} from "@/features/chat/message/application/renderUnitAnchor";
import { unitSeam } from "@/features/chat/message/application/renderUnitRhythm";
import { seamStep } from "@/features/chat/message/ui/messageStyles";
import { DelegatedRunLink } from "@/features/chat/message/ui/DelegatedRunLink";
import { NarrativeWave } from "@/features/chat/message/ui/NarrativeWave";
export type { BlockCtx } from "@/features/chat/message/ui/blockContext";

const styles = stylex.create({
  arrival: { position: "relative" },
  // Delegated runs sit under the tool's label column as its children.
  delegatedRuns: { paddingInlineStart: space.s6 },
});

// The Run an awaitable item resumes, once the Session reports it waiting.
function resumeRun(facts: TurnFacts, itemId: string | undefined): string | undefined {
  return itemId === undefined ? undefined : facts.awaiting.get(itemId);
}

function recordedDecision(facts: TurnFacts, itemId: string | undefined) {
  return itemId === undefined ? undefined : facts.toolCalls[itemId]?.approvalDecision;
}

function renderToolBlock(
  block: Extract<ContentBlock, { kind: "tool" }>,
  facts: TurnFacts,
  ctx: BlockCtx,
  inlineRenderer: ToolResultRendererSpec | undefined,
) {
  const tool = facts.toolCalls[block.itemId];
  if (!tool) return null;
  const delegatedRuns = facts.delegatedRuns[block.itemId] ?? [];
  const delegated = delegatedRuns.length > 0;
  return (
    <div
      id={block.itemId}
      key={block.itemId}
      role={delegated ? "group" : undefined}
      aria-label={delegated ? tool.fn : undefined}
    >
      <ToolCard
        tool={tool}
        inlineRenderer={inlineRenderer}
        expanded={ctx.expandedIds.has(block.itemId)}
        onExpandedChange={(expanded) => ctx.onExpandedChange(block.itemId, expanded)}
      />
      {delegated && (
        <div {...stylex.props(styles.delegatedRuns)}>
          {delegatedRuns.map((narrative, index) => (
            <DelegatedRunLink
              key={narrative.run.id}
              run={narrative.run}
              ordinal={index + 1}
              siblingCount={delegatedRuns.length}
              size="sm"
            />
          ))}
        </div>
      )}
    </div>
  );
}

export function renderBlock(
  block: ContentBlock,
  key: number,
  facts: TurnFacts,
  ctx: BlockCtx,
  inlineRenderer?: ToolResultRendererSpec,
) {
  switch (block.kind) {
    case "text":
      return (
        <div key={key} data-slot="message-text">
          <MarkdownMessage text={block.text} streaming={block.status === "running"} />
        </div>
      );

    case "image":
      return <ImageBlock key={key} mime={block.mime} data={block.data} />;

    case "reference":
      return <ReferenceBlock key={key} label={block.label} text={block.text} />;

    case "tool":
      return renderToolBlock(block, facts, ctx, inlineRenderer);

    case "reasoning":
      return <ReasoningBlock key={key} text={block.text} status={block.status} />;

    case "approval":
      return (
        <ApprovalCard
          key={block.itemId ?? key}
          sessionId={ctx.sessionId}
          resumeRunId={resumeRun(facts, block.itemId)}
          decision={recordedDecision(facts, block.itemId)}
          toolName={block.toolName}
          category={block.category}
          cmd={block.command}
          reason={block.reason}
          itemId={block.itemId}
          args={block.args}
          rememberable={block.rememberable}
        />
      );

    case "question":
      return (
        <QuestionCard
          key={block.itemId ?? key}
          sessionId={ctx.sessionId}
          status={block.status}
          resumeRunId={resumeRun(facts, block.itemId)}
          itemId={block.itemId}
          questions={block.questions}
          answered={block.answered}
          answers={block.answers}
        />
      );

    case "compaction":
      return <CompactionBlock key={key} summary={block.summary} />;
  }
}

function renderUnit(
  messageId: string,
  unit: MessageRenderUnit,
  facts: TurnFacts,
  ctx: BlockCtx,
  standaloneResults?: ReadonlyMap<string, StandaloneToolResult>,
) {
  if (unit.kind === "wave")
    return (
      <NarrativeWave
        messageId={messageId}
        units={unit.units}
        facts={facts}
        ctx={ctx}
        renderUnit={renderUnit}
      />
    );
  if (unit.kind === "toolGroup") {
    return (
      <ToolGroup
        tools={unit.tools}
        expandedIds={ctx.expandedIds}
        onExpandedChange={ctx.onExpandedChange}
        superseded={unit.superseded}
      />
    );
  }
  const inlineRenderer =
    unit.inlineResult && unit.block.kind === "tool"
      ? standaloneResults?.get(unit.block.itemId)?.renderer
      : undefined;
  return renderBlock(unit.block, unit.index, facts, ctx, inlineRenderer);
}

const standingTool = (name: string) =>
  lookupExtensionByKey(TOOL_STANDING_SURFACE, name) !== undefined;

export function renderMessageBlocks(
  { message, facts }: Pick<TranscriptRow, "message" | "facts">,
  ctx: BlockCtx,
  standaloneToolResult: StandaloneToolResultFor,
  answerFollows = false,
) {
  const blocks = narratedBlocks(message.blocks, facts.toolCalls, standingTool);
  const standaloneResults = new Map<string, StandaloneToolResult>();
  for (const block of blocks) {
    if (block.kind !== "tool") continue;
    const tool = facts.toolCalls[block.itemId];
    const result = tool && standaloneToolResult(tool);
    if (result) standaloneResults.set(block.itemId, result);
  }
  const units = messageBlockRenderUnits(
    blocks,
    facts.toolCalls,
    facts.awaiting,
    answerFollows,
    new Set(Object.keys(facts.delegatedRuns)),
    (tool) => {
      const result = standaloneResults.get(tool.id);
      if (result) return result.renderer ? "inline" : "standalone";
    },
  ).filter((unit) => unit.kind !== "block" || unit.block !== ctx.questionInComposer);
  return units.map((unit, index) => {
    const anchor = renderUnitAnchor(message.id, unit);
    return (
      <motion.div
        key={renderUnitKey(message.id, unit)}
        {...stepEnter}
        {...{ [BLOCK_ANCHOR_ATTR]: anchor }}
        className={
          stylex.props(styles.arrival, seamStep[unitSeam(units[index - 1], unit) ?? "none"])
            .className
        }
      >
        {renderUnit(message.id, unit, facts, ctx, standaloneResults)}
      </motion.div>
    );
  });
}

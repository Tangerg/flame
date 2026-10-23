import * as stylex from "@stylexjs/stylex";
import { motion } from "motion/react";
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
import { ToolCard } from "@/features/chat/public/tools/rendering";
import { ToolGroup } from "@/features/chat/public/tools/rendering";
import { lookupExtensionByKey } from "@/extensions/selectors/extensions";
import { TOOL_STANDING_SURFACE } from "@/features/chat/public/tools/types";
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

const styles = stylex.create({ arrival: { position: "relative" } });

export function renderBlock(block: ContentBlock, key: number, facts: TurnFacts, ctx: BlockCtx) {
  switch (block.kind) {
    case "text":
      return (
        <div key={key} data-slot="message-text">
          {ctx.textReveal === "instant" ? (
            <MarkdownMessage text={block.text} reveal="instant" />
          ) : (
            <MarkdownMessage
              text={block.text}
              streaming={block.status === "running"}
              reveal={ctx.textReveal}
            />
          )}
        </div>
      );

    case "image":
      return <ImageBlock key={key} mime={block.mime} data={block.data} />;

    case "tool": {
      const tool = facts.toolCalls[block.toolCallId];
      if (!tool) return null;
      const delegatedRuns = facts.delegatedRuns[block.toolCallId] ?? [];
      return (
        <div
          id={block.toolCallId}
          key={block.toolCallId}
          role={delegatedRuns.length > 0 ? "group" : undefined}
          aria-label={delegatedRuns.length > 0 ? tool.fn : undefined}
        >
          <ToolCard
            tool={tool}
            expanded={ctx.expandedIds.has(block.toolCallId)}
            onToggleExpand={() => ctx.onToggleExpand(block.toolCallId)}
          />
          {delegatedRuns.map((narrative, index) => (
            <DelegatedRunLink
              key={narrative.run.id}
              run={narrative.run}
              ordinal={index + 1}
              siblingCount={delegatedRuns.length}
            />
          ))}
        </div>
      );
    }

    case "reasoning":
      return <ReasoningBlock key={key} text={block.text} status={block.status} />;

    case "approval":
      return (
        <ApprovalCard
          key={block.itemId ?? key}
          sessionId={ctx.sessionId}
          resumeRunId={block.itemId === undefined ? undefined : facts.awaiting.get(block.itemId)}
          decision={
            block.itemId === undefined ? undefined : facts.toolCalls[block.itemId]?.approvalDecision
          }
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
          resumeRunId={block.itemId === undefined ? undefined : facts.awaiting.get(block.itemId)}
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

function renderUnit(messageId: string, unit: MessageRenderUnit, facts: TurnFacts, ctx: BlockCtx) {
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
        onToggleExpand={ctx.onToggleExpand}
        superseded={unit.superseded}
      />
    );
  }
  return renderBlock(unit.block, unit.index, facts, ctx);
}

const standingTool = (name: string) =>
  lookupExtensionByKey(TOOL_STANDING_SURFACE, name) !== undefined;

export function renderMessageBlocks(
  { message, facts }: Pick<TranscriptRow, "message" | "facts">,
  ctx: BlockCtx,
  answerFollows = false,
) {
  const units = messageBlockRenderUnits(
    narratedBlocks(message.blocks, facts.toolCalls, standingTool),
    facts.toolCalls,
    facts.awaiting,
    answerFollows,
    new Set(Object.keys(facts.delegatedRuns)),
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
        {renderUnit(message.id, unit, facts, ctx)}
      </motion.div>
    );
  });
}

import * as stylex from "@stylexjs/stylex";
import { useId } from "react";
import { type ToolCall } from "@/features/agent/public/sessionView";
import { AgentActivityDisclosure } from "@/ui/agent/index";
import { type ToolMetaItem } from "@/features/agent/public/messagePresentation";
import { useT } from "@/lib/i18n/index";
import { toolCardModel } from "@/features/chat/tools/application/toolCardModel";
import { toolCallIconFor } from "@/features/agent/public/toolIcon";
import { InlineToolResult, ToolPreview } from "@/features/chat/tools/ui/ToolPreview";
import { type ToolResultRendererSpec } from "@/features/chat/public/tools/types";
import { ToolText } from "@/ui/agent/index";
import { space, type as typeStep } from "@flame/ui/tokens.stylex";
import { toolMetaInk } from "@/features/chat/tools/ui/toolMetaInk";
import {
  ToolActionFailureLine,
  ToolFailureLine,
  ToolStatusMarks,
  ToolRowActions,
  useToolRowActions,
} from "@/features/chat/tools/ui/ToolRowParts";
interface Props {
  tool: ToolCall;
  inlineRenderer?: ToolResultRendererSpec;
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
}

const tc = stylex.create({
  details: { marginBlockStart: space.s2 },
  full: { width: "100%" },
  meta: { fontVariantNumeric: "tabular-nums" },
  secondaryMeta: {
    display: { default: "none", "@container (min-width: 24rem)": "inline" },
  },
  status: {
    display: "flex",
    flexShrink: 0,
    alignItems: "center",
    gap: space.s1_5,
  },
});

export function ToolCard({ tool, inlineRenderer, expanded, onExpandedChange }: Props) {
  const t = useT();
  const model = toolCardModel(t, tool);
  const summaryId = useId();
  const { controls: actions, feedback } = useToolRowActions(tool, summaryId);
  const stackDetail =
    !inlineRenderer && expanded && model.disclosable && model.detail?.kind === "path";

  return (
    <>
      {inlineRenderer && <InlineToolResult tool={tool} renderer={inlineRenderer} />}
      <AgentActivityDisclosure
        data-tool={tool.name}
        className={inlineRenderer ? stylex.props(tc.details).className : undefined}
        icon={toolCallIconFor(tool)}
        shell="line"
        summaryId={summaryId}
        contentInset="rows"
        label={
          inlineRenderer ? (
            t("toolInspector.details")
          ) : (
            <ToolText value={model.intent.label} styles={tc.full} wrap={expanded} />
          )
        }
        detail={
          !inlineRenderer && model.detail ? (
            <ToolText value={model.detail} styles={tc.full} wrap={stackDetail} />
          ) : undefined
        }
        detailPlacement={stackDetail ? "below" : undefined}
        trailing={
          <>
            <ToolMeta items={model.metaItems} />
            <ToolStatusMarks model={model} />
          </>
        }
        actions={actions.length > 0 ? <ToolRowActions>{actions}</ToolRowActions> : undefined}
        feedback={<ToolActionFailureLine feedback={feedback} summaryId={summaryId} />}
        open={expanded}
        onOpenChange={onExpandedChange}
      >
        {model.disclosable ? (
          <ToolPreview tool={tool} raw={inlineRenderer !== undefined} />
        ) : undefined}
      </AgentActivityDisclosure>
      <ToolFailureLine model={model} />
    </>
  );
}

function ToolMeta({ items }: { items: ToolMetaItem[] }) {
  if (items.length === 0) return null;

  return (
    <span {...stylex.props(tc.status)}>
      {items.map((item) => (
        <span
          key={item.id}
          data-tone={item.tone}
          {...stylex.props(
            tc.meta,
            item.tone !== "negative" && tc.secondaryMeta,
            toolMetaInk.card[item.tone],
            typeStep.uiXs,
          )}
        >
          {item.label}
        </span>
      ))}
    </span>
  );
}

import * as stylex from "@stylexjs/stylex";
import { useId } from "react";
import { type ToolCall } from "@/features/agent/public/sessionView";
import { Icon } from "@/ui/index";
import { Pressable } from "@/ui/index";
import { reveal } from "@/ui/index";
import { chevron } from "@/ui/index";
import { DiffStat, vocab } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import {
  headlineToolMetaItem,
  toolCardModel,
} from "@/features/chat/tools/application/toolCardModel";
import { toolCallIconFor } from "@/features/agent/public/toolIcon";
import { ToolPreview } from "@/features/chat/tools/ui/ToolPreview";
import { ToolText } from "@/ui/agent/index";
import { color, space, type as typeStep } from "@flame/ui/tokens.stylex";
import { toolMetaInk } from "@/features/chat/tools/ui/toolMetaInk";
import {
  ToolActionFailureLine,
  ToolFailureLine,
  ToolStatusMarks,
  useToolRowActions,
} from "@/features/chat/tools/ui/ToolRowParts";
interface Props {
  tool: ToolCall;
  expanded: boolean;
  onToggleExpand: () => void;
}

const gm = stylex.create({
  body: { paddingBlock: space.s1_5 },
  inherit: { color: "inherit" },
  line: { display: "flex", minWidth: 0, alignItems: "center", gap: space.s1_5 },
  row: {
    display: "flex",
    minWidth: 0,
    flex: 1,
    alignItems: "baseline",
    gap: space.s2,
    paddingBlock: space.s0_5,
    textAlign: "left",
    color: { default: color.fgMuted, ":hover": color.fg },
  },
  mark: {
    display: "grid",
    flexShrink: 0,
    alignSelf: "center",
    placeItems: "center",
    width: space.s4,
    height: space.s4,
  },
  chevron: {
    pointerEvents: "none",
    display: "flex",
    alignSelf: "center",
    marginInlineStart: "calc(var(--spacing) * -1)",
  },
  actions: { display: "flex", flexShrink: 0, alignItems: "center" },
  rowExpanded: { color: color.fg },
  meta: { flexShrink: 0, fontVariantNumeric: "tabular-nums" },
});

export function ToolGroupMember({ tool, expanded, onToggleExpand }: Props) {
  const t = useT();
  const model = toolCardModel(t, tool);
  const headline = headlineToolMetaItem(model.metaItems);

  const summaryId = useId();
  const bodyId = useId();
  const { controls: actions, feedback } = useToolRowActions(tool, summaryId);
  // Disclosability is one fact: a denied tool has no preview, so the row is a
  // static summary, never a disclosure that claims aria-expanded with no panel.
  const disclosable = !model.denied;
  const disclosed = disclosable && expanded;
  const rowClassName = stylex.props(gm.row, typeStep.uiSm, disclosed && gm.rowExpanded).className;

  const summary = (
    <>
      <span aria-hidden {...stylex.props(gm.mark, vocab.faint)}>
        <Icon name={toolCallIconFor(tool)} size="sm" />
      </span>
      <ToolText value={model.intent.label} styles={[vocab.hold, gm.inherit]} />
      {disclosable && (
        <span
          aria-hidden
          {...stylex.props(chevron.base, gm.chevron, vocab.faint, !disclosed && chevron.shut)}
        >
          <Icon name="chevron-down" size="xs" />
        </span>
      )}
      {model.detail && <ToolText value={model.detail} styles={[vocab.fill, vocab.faint]} />}
      {model.diffStat && <DiffStat added={model.diffStat.added} removed={model.diffStat.removed} />}
      {headline && (
        <span
          data-tone={headline.tone}
          {...stylex.props(gm.meta, toolMetaInk.member[headline.tone], typeStep.uiXs)}
        >
          {headline.label}
        </span>
      )}
      <ToolStatusMarks model={model} />
    </>
  );

  return (
    <div>
      <div {...stylex.props(gm.line, reveal.host)}>
        {disclosable ? (
          <Pressable
            id={summaryId}
            data-tool={tool.name}
            type="button"
            aria-expanded={expanded}
            aria-controls={disclosed ? bodyId : undefined}
            onClick={onToggleExpand}
            className={rowClassName}
          >
            {summary}
          </Pressable>
        ) : (
          <div id={summaryId} data-tool={tool.name} className={rowClassName}>
            {summary}
          </div>
        )}
        {actions.length > 0 && <span {...stylex.props(gm.actions)}>{actions}</span>}
      </div>
      <ToolActionFailureLine feedback={feedback} summaryId={summaryId} />
      <ToolFailureLine model={model} />
      {disclosed && (
        <div id={bodyId} {...stylex.props(gm.body)}>
          <ToolPreview tool={tool} />
        </div>
      )}
    </div>
  );
}

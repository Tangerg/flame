import * as stylex from "@stylexjs/stylex";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { Icon, Pressable, reveal } from "@/ui";
import { DiffStat, vocab } from "@flame/ui";
import { useT } from "@/lib/i18n";
import { headlineToolMetaItem, toolCardModel } from "../application/toolCardModel";
import { toolCallIconFor } from "@/plugins/builtin/agent/public/toolIcon";
import { ToolPreview } from "./ToolPreview";
import { ToolText } from "@/ui/agent";
import { color, space, type as typeStep } from "@flame/ui/tokens.stylex";
import { toolMetaInk } from "./toolMetaInk";
import { ToolFailureLine, ToolStatusMarks, useToolRowActions } from "./ToolRowParts";

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
  actions: { display: "flex", flexShrink: 0, alignItems: "center" },
  rowExpanded: { color: color.fg },
  meta: { flexShrink: 0, fontVariantNumeric: "tabular-nums" },
});

export function ToolGroupMember({ tool, expanded, onToggleExpand }: Props) {
  const t = useT();
  const model = toolCardModel(t, tool);
  const headline = headlineToolMetaItem(model.metaItems);

  const actions = useToolRowActions(tool);

  return (
    <div>
      <div {...stylex.props(gm.line, reveal.host)}>
        <Pressable
          data-tool={tool.name}
          type="button"
          aria-expanded={expanded}
          onClick={onToggleExpand}
          className={stylex.props(gm.row, typeStep.uiSm, expanded && gm.rowExpanded).className}
        >
          <span aria-hidden {...stylex.props(gm.mark, vocab.faint)}>
            <Icon name={toolCallIconFor(tool)} size="sm" />
          </span>
          <ToolText value={model.intent.label} styles={[vocab.hold, gm.inherit]} />
          {model.detail && <ToolText value={model.detail} styles={[vocab.fill, vocab.faint]} />}
          {model.diffStat && (
            <DiffStat added={model.diffStat.added} removed={model.diffStat.removed} />
          )}
          {headline && (
            <span
              data-tone={headline.tone}
              {...stylex.props(gm.meta, toolMetaInk.member[headline.tone], typeStep.uiXs)}
            >
              {headline.label}
            </span>
          )}
          <ToolStatusMarks model={model} />
        </Pressable>
        {actions.length > 0 && <span {...stylex.props(gm.actions)}>{actions}</span>}
      </div>
      <ToolFailureLine model={model} />
      {expanded && !model.denied && (
        <div {...stylex.props(gm.body)}>
          <ToolPreview tool={tool} />
        </div>
      )}
    </div>
  );
}

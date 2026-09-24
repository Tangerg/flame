import * as stylex from "@stylexjs/stylex";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { DiffStat, Icon, Pressable, reveal, vocab } from "@/ui";
import { useT } from "@/lib/i18n";
import { headlineToolMetaItem, toolCardModel } from "../application/toolCardModel";
import { toolCallIconFor } from "@/plugins/builtin/agent/public/toolIcon";
import { ToolPreview } from "./ToolPreview";
import { ToolText } from "@/ui/agent";
import { color, face, space, type as typeStep } from "@/styles/tokens.stylex";
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
    gap: space.s1_5,
    paddingBlock: space.s0_5,
    textAlign: "left",
    color: { default: color.fgMuted, ":hover": color.fg },
  },
  actions: { display: "flex", flexShrink: 0, alignItems: "center" },
  rowExpanded: { color: color.fg },
  meta: { flexShrink: 0 },
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
          className={stylex.props(gm.row, expanded && gm.rowExpanded).className}
        >
          <Icon
            name={toolCallIconFor(tool)}
            size="xs"
            className={stylex.props(vocab.hold, vocab.muted).className}
          />
          <ToolText value={model.intent.label} styles={[vocab.hold, gm.inherit, typeStep.uiMd]} />
          {model.detail && (
            <ToolText value={model.detail} styles={[vocab.fill, vocab.faint, typeStep.uiMd]} />
          )}
          {model.diffStat && (
            <DiffStat added={model.diffStat.added} removed={model.diffStat.removed} />
          )}
          {headline && (
            <span
              data-tone={headline.tone}
              {...stylex.props(
                gm.meta,
                toolMetaInk.member[headline.tone],
                typeStep.uiXs,
                face.mono,
              )}
            >
              {headline.label}
            </span>
          )}
          <ToolStatusMarks model={model} />
        </Pressable>
        <span {...stylex.props(gm.actions)}>{actions}</span>
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

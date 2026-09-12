import * as stylex from "@stylexjs/stylex";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { DiffStat, Icon, Pressable, vocab } from "@/ui";
import { useT } from "@/lib/i18n";
import { headlineToolMetaItem, toolCardModel } from "../application/toolCardModel";
import { toolCallIconFor } from "../public/toolIcon";
import { ToolPreview } from "./ToolPreview";
import { ToolText } from "./ToolText";
import { color, face, space, type as typeStep } from "@/styles/tokens.stylex";
import { toolMetaInk } from "./toolMetaInk";

interface Props {
  tool: ToolCall;
  expanded: boolean;
  onToggleExpand: () => void;
}

const gm = stylex.create({
  body: { paddingBlock: space.s1_5 },
  // The member takes the row's ink, which the row itself decides from its state.
  inherit: { color: "inherit" },
  row: {
    display: "flex",
    width: "100%",
    minWidth: 0,
    alignItems: "baseline",
    gap: space.s1_5,
    paddingBlock: space.s0_5,
    textAlign: "left",
    color: { default: color.fgMuted, ":hover": color.fg },
  },
  rowExpanded: { color: color.fg },
  meta: { flexShrink: 0 },
});

export function ToolGroupMember({ tool, expanded, onToggleExpand }: Props) {
  const t = useT();
  const model = toolCardModel(t, tool);
  const headline = headlineToolMetaItem(model.metaItems);

  return (
    <div>
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
        <ToolText
          value={model.intent.label}
          className={stylex.props(vocab.hold, gm.inherit, typeStep.uiSm).className}
        />
        {model.detail && (
          <ToolText
            value={model.detail}
            className={stylex.props(vocab.fill, vocab.faint, typeStep.uiSm, face.mono).className}
          />
        )}
        {model.diffStat && (
          <DiffStat added={model.diffStat.added} removed={model.diffStat.removed} />
        )}
        {headline && (
          <span
            // The tone is the decision and the ink is its rendering. A test that reads the ink
            // back through a class name breaks when the ink moves and says nothing when the
            // DECISION regresses, which is the failure this row has actually had.
            data-tone={headline.tone}
            {...stylex.props(gm.meta, toolMetaInk.member[headline.tone], typeStep.ui2xs, face.mono)}
          >
            {headline.label}
          </span>
        )}
      </Pressable>
      {expanded && (
        <div {...stylex.props(gm.body)}>
          <ToolPreview tool={tool} />
        </div>
      )}
    </div>
  );
}

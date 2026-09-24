import * as stylex from "@stylexjs/stylex";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { DiffStat } from "@/ui";
import { AgentActivityDisclosure } from "@/ui/agent";
import { type ToolMetaItem } from "@/plugins/builtin/agent/public/messagePresentation";
import { useT } from "@/lib/i18n";
import { toolCardModel } from "../application/toolCardModel";
import { toolCallIconFor } from "@/plugins/builtin/agent/public/toolIcon";
import { ToolPreview } from "./ToolPreview";
import { ToolText } from "@/ui/agent";
import { face, space, type as typeStep, weight } from "@/styles/tokens.stylex";
import { toolMetaInk } from "./toolMetaInk";
import { ToolFailureLine, ToolStatusMarks, useToolRowActions } from "./ToolRowParts";

interface Props {
  tool: ToolCall;
  expanded: boolean;
  onToggleExpand: () => void;
}

const tc = stylex.create({
  full: { width: "100%" },
  meta: { fontWeight: weight.medium },
  status: {
    display: { default: "none", "@container (min-width: 24rem)": "flex" },
    flexShrink: 0,
    alignItems: "center",
    gap: space.s1_5,
  },
});

export function ToolCard({ tool, expanded, onToggleExpand }: Props) {
  const t = useT();
  const model = toolCardModel(t, tool);
  const actions = useToolRowActions(tool);

  return (
    <>
      <AgentActivityDisclosure
        data-tool={tool.name}
        icon={toolCallIconFor(tool)}
        shell="line"
        contentInset="rows"
        label={<ToolText value={model.intent.label} styles={tc.full} />}
        detail={model.detail ? <ToolText value={model.detail} styles={tc.full} /> : undefined}
        trailing={
          <>
            {model.diffStat && (
              <DiffStat added={model.diffStat.added} removed={model.diffStat.removed} />
            )}
            <ToolMeta items={model.metaItems} />
            <ToolStatusMarks model={model} />
          </>
        }
        actions={actions}
        open={expanded}
        onToggle={onToggleExpand}
      >
        {model.denied ? undefined : <ToolPreview tool={tool} />}
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
          {...stylex.props(tc.meta, toolMetaInk.card[item.tone], typeStep.uiXs, face.mono)}
        >
          {item.label}
        </span>
      ))}
    </span>
  );
}

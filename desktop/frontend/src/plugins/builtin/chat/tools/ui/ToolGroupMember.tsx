import * as stylex from "@stylexjs/stylex";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { DiffStat, Icon, Pressable, vocab } from "@/ui";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { headlineToolMetaItem, toolCardModel } from "../application/toolCardModel";
import { toolCallIconFor } from "../public/toolIcon";
import { ToolPreview } from "./ToolPreview";
import { ToolText } from "./ToolText";
import { face, space, type as typeStep } from "@/styles/tokens.stylex";

interface Props {
  tool: ToolCall;
  expanded: boolean;
  onToggleExpand: () => void;
}

const gm = stylex.create({
  body: { paddingBlock: space.s1_5 },
  // The member takes the row's ink, which the row itself decides from its state.
  inherit: { color: "inherit" },
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
        className={cn(
          "flex w-full min-w-0 items-baseline gap-1.5 py-0.5 text-left text-fg-muted",
          "hover:text-fg",
          expanded && "text-fg",
        )}
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
            className={cn(
              "shrink-0 font-mono text-ui-2xs",
              headline.tone === "negative" ? "text-negative" : "text-fg-faint",
            )}
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

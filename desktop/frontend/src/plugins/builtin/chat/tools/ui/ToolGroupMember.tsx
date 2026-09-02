import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { DiffStat, Icon, Pressable } from "@/ui";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { headlineToolMetaItem, toolCardModel } from "../application/toolCardModel";
import { toolCallIconFor } from "../public/toolIcon";
import { ToolPreview } from "./ToolPreview";
import { ToolText } from "./ToolText";

interface Props {
  tool: ToolCall;
  expanded: boolean;
  onToggleExpand: () => void;
}

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
        <Icon name={toolCallIconFor(tool)} size="xs" className="shrink-0 text-fg-muted" />
        <ToolText value={model.intent.label} className="shrink-0 text-ui-sm text-inherit" />
        {model.detail && (
          <ToolText
            value={model.detail}
            className="min-w-0 flex-1 font-mono text-ui-sm text-fg-faint"
          />
        )}
        {model.diffStat && (
          <DiffStat added={model.diffStat.added} removed={model.diffStat.removed} />
        )}
        {headline && (
          <span
            className={cn(
              "shrink-0 font-mono text-ui-2xs tabular-nums",
              headline.tone === "negative" ? "text-negative" : "text-fg-faint",
            )}
          >
            {headline.label}
          </span>
        )}
      </Pressable>
      {expanded && (
        <div className="pt-1.5 pb-1.5">
          <ToolPreview tool={tool} />
        </div>
      )}
    </div>
  );
}

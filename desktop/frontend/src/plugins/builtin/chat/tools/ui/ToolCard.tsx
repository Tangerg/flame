import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { DiffStat, IconButton, StatusDot, knownIconName } from "@/ui";
import { AgentActivityDisclosure } from "@/ui/agent";
import { type ToolMetaItem } from "@/plugins/builtin/agent/public/messagePresentation";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import {
  lookupToolActionOwner,
  lookupToolViewOpenerOwner,
  reportPluginError,
  TOOL_ACTION,
  TOOL_VIEW_OPENER,
  useExtensionPoint,
} from "@/plugins/sdk";
import { toolCardActions, toolCardModel, toolCardViewOpener } from "../application/toolCardModel";
import { toolCallIconFor } from "../public/toolIcon";
import { ToolPreview } from "./ToolPreview";
import { ToolText } from "./ToolText";

interface Props {
  tool: ToolCall;
  expanded: boolean;
  onToggleExpand: () => void;
}

export function ToolCard({ tool, expanded, onToggleExpand }: Props) {
  const t = useT();
  const model = toolCardModel(t, tool);
  const allActions = useExtensionPoint(TOOL_ACTION);
  const allViewOpeners = useExtensionPoint(TOOL_VIEW_OPENER);
  const actions = toolCardActions(tool, allActions);
  const viewOpener = toolCardViewOpener(tool, allViewOpeners);
  const onOpenView = viewOpener
    ? () => {
        void Promise.resolve(viewOpener.open(tool)).catch((err) => {
          const owner = lookupToolViewOpenerOwner(viewOpener.id) ?? "unknown";
          console.error(`[plugin] tool view opener ${viewOpener.id} threw:`, err);
          reportPluginError(owner, "command", err, `tool view opener: ${viewOpener.id}`);
        });
      }
    : undefined;

  return (
    <AgentActivityDisclosure
      data-tool={tool.name}
      data-status={tool.status}
      icon={toolCallIconFor(tool)}
      // Every invocation stays on the work-narrative line and takes the neutral tone,
      // whatever its safety class or outcome: the material result earns a surface only once
      // the row is opened, and colouring the identity glyph turns a failure or a refusal
      // back into a status card.
      shell="line"
      label={<ToolText value={model.intent.label} className="w-full" />}
      detail={
        model.detail ? <ToolText value={model.detail} className="w-full font-mono" /> : undefined
      }
      trailing={
        <>
          {model.diffStat && (
            <DiffStat added={model.diffStat.added} removed={model.diffStat.removed} />
          )}
          <ToolMeta items={model.metaItems} />
          {model.running && <StatusDot tone="running" />}
          {model.denied && (
            <span data-slot="tool-status" className="font-sans text-ui-xs text-fg-muted">
              {t("tool.state.denied")}
            </span>
          )}
        </>
      }
      actions={actions.map((action) => (
        <IconButton
          key={action.id}
          data-reveal="hover"
          icon={knownIconName(action.icon) ?? "tool"}
          size="xs"
          quiet
          title={t(action.title)}
          onClick={(event) => {
            event.stopPropagation();
            void Promise.resolve(action.run(tool)).catch((err) => {
              const owner = lookupToolActionOwner(action.id) ?? "unknown";
              console.error(`[plugin] tool action ${action.id} threw:`, err);
              reportPluginError(owner, "command", err, `tool action: ${action.id}`);
            });
          }}
          className="opacity-0 transition-opacity group-hover/activity-header:opacity-100 focus-visible:opacity-100"
        />
      ))}
      open={expanded}
      onToggle={onToggleExpand}
    >
      <ToolPreview tool={tool} onOpenView={onOpenView} />
    </AgentActivityDisclosure>
  );
}

function ToolMeta({ items }: { items: ToolMetaItem[] }) {
  if (items.length === 0) return null;

  return (
    <span className="hidden shrink-0 items-center gap-1.5 @sm:flex">
      {items.map((item) => (
        <span
          key={item.id}
          className={cn(
            "font-mono text-ui-xs font-medium tabular-nums",
            toolMetaToneClass(item.tone),
          )}
        >
          {item.label}
        </span>
      ))}
    </span>
  );
}

function toolMetaToneClass(tone: ToolMetaItem["tone"]): string {
  return tone === "negative" ? "text-negative" : "text-fg-muted";
}

import * as stylex from "@stylexjs/stylex";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { DiffStat, IconButton, knownIconName, reveal, StatusDot, vocab } from "@/ui";
import { AgentActivityDisclosure } from "@/ui/agent";
import { type ToolMetaItem } from "@/plugins/builtin/agent/public/messagePresentation";
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
import { face, space, type as typeStep, weight } from "@/styles/tokens.stylex";
import { toolMetaInk } from "./toolMetaInk";

interface Props {
  tool: ToolCall;
  expanded: boolean;
  onToggleExpand: () => void;
}

const tc = stylex.create({
  full: { width: "100%" },
  sans: { fontFamily: "var(--font-sans)" },
  // The status only appears once the card is wide enough for it beside the label.
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
      icon={toolCallIconFor(tool)}
      // Every invocation stays on the work-narrative line and takes the neutral tone,
      // whatever its safety class or outcome: the material result earns a surface only once
      // the row is opened, and colouring the identity glyph turns a failure or a refusal
      // back into a status card.
      shell="line"
      contentClassName="py-1.5"
      label={<ToolText value={model.intent.label} className={stylex.props(tc.full).className} />}
      detail={
        model.detail ? (
          <ToolText value={model.detail} className={stylex.props(tc.full, face.mono).className} />
        ) : undefined
      }
      trailing={
        <>
          {model.diffStat && (
            <DiffStat added={model.diffStat.added} removed={model.diffStat.removed} />
          )}
          <ToolMeta items={model.metaItems} />
          {model.running && <StatusDot tone="running" />}
          {model.denied && (
            <span data-slot="tool-status" {...stylex.props(tc.sans, vocab.muted, typeStep.uiXs)}>
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
          className={stylex.props(reveal.shown).className}
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

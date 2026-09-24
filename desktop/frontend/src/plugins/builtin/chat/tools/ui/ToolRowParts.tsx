import * as stylex from "@stylexjs/stylex";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { IconButton, knownIconName, reveal, StatusDot, vocab } from "@/ui";
import { useT } from "@/lib/i18n";
import {
  lookupToolActionOwner,
  lookupToolViewOpenerOwner,
  reportPluginError,
  TOOL_ACTION,
  TOOL_VIEW_OPENER,
  useExtensionPoint,
} from "@/plugins/sdk";
import {
  type ToolCardModel,
  toolCardActions,
  toolCardViewOpener,
} from "../application/toolCardModel";
import { color, space, type as typeStep } from "@/styles/tokens.stylex";

const styles = stylex.create({
  sans: { fontFamily: "var(--font-sans)" },
  failure: { flexShrink: 0, color: color.negative },
  openSlot: { flexShrink: 0, width: "var(--control-height-xs)" },
  failureLine: {
    marginTop: space.s0_5,
    marginLeft: "calc(var(--spacing) * 5.5)",
    overflowWrap: "anywhere",
    color: color.fg,
  },
});

export function ToolStatusMarks({ model }: { model: ToolCardModel }) {
  const t = useT();
  return (
    <>
      {model.running && <StatusDot tone="running" />}
      {model.error !== undefined && (
        <span
          data-slot="tool-status"
          data-tone="negative"
          {...stylex.props(styles.sans, styles.failure, typeStep.uiXs)}
        >
          {t("tool.state.failed")}
        </span>
      )}
      {model.denied && (
        <span data-slot="tool-status" {...stylex.props(styles.sans, vocab.muted, typeStep.uiXs)}>
          {t("tool.state.denied")}
        </span>
      )}
    </>
  );
}

export function ToolFailureLine({ model }: { model: ToolCardModel }) {
  if (model.error === undefined) return null;
  return (
    <p data-slot="tool-error" {...stylex.props(styles.failureLine, typeStep.uiSm)}>
      {model.error}
    </p>
  );
}

export function useToolRowActions(tool: ToolCall) {
  const t = useT();
  const actions = toolCardActions(tool, useExtensionPoint(TOOL_ACTION));
  const opener = toolCardViewOpener(tool, useExtensionPoint(TOOL_VIEW_OPENER));
  return [
    ...actions.map((action) => (
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
    )),
    opener ? (
      <IconButton
        key="open-view"
        data-reveal="hover"
        data-slot="tool-open-view"
        icon="open"
        size="xs"
        quiet
        title={t(opener.label(tool))}
        onClick={(event) => {
          event.stopPropagation();
          void Promise.resolve(opener.open(tool)).catch((err) => {
            const owner = lookupToolViewOpenerOwner(opener.id) ?? "unknown";
            console.error(`[plugin] tool view opener ${opener.id} threw:`, err);
            reportPluginError(owner, "command", err, `tool view opener: ${opener.id}`);
          });
        }}
        className={stylex.props(reveal.shown).className}
      />
    ) : (
      <span key="open-view" aria-hidden {...stylex.props(styles.openSlot)} />
    ),
  ];
}

import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { type ToolCall } from "@/features/agent/public/sessionView";
import { knownIconName } from "@/ui/index";
import { reveal } from "@/ui/index";
import { StatusDot } from "@/ui/index";
import { IconButton, vocab } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import {
  lookupToolActionOwner,
  lookupToolViewOpenerOwner,
  TOOL_ACTION,
  TOOL_VIEW_OPENER,
} from "@/features/chat/public/tools/types";
import { reportPluginError } from "@/extensions/errors";
import { useExtensionPoint } from "@/extensions/selectors/extensions";
import {
  type ToolCardModel,
  toolCardActions,
  toolCardViewOpener,
} from "@/features/chat/tools/application/toolCardModel";
import { color, space, type as typeStep } from "@flame/ui/tokens.stylex";
const styles = stylex.create({
  sans: { fontFamily: "var(--font-sans)" },
  failure: { flexShrink: 0, color: color.negative },
  failureLine: {
    marginTop: space.s0_5,
    marginLeft: space.s6,
    overflowWrap: "anywhere",
    color: color.fg,
  },
});

export function ToolStatusMarks({ model }: { model: ToolCardModel }) {
  const t = useT();
  return (
    <>
      {model.running && <StatusDot tone="running" />}
      {model.failed && (
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

export function useToolRowActions(tool: ToolCall, summaryId: string) {
  const t = useT();
  const actions = toolCardActions(tool, useExtensionPoint(TOOL_ACTION));
  const opener = toolCardViewOpener(tool, useExtensionPoint(TOOL_VIEW_OPENER));
  const controls = actions.map((action) => (
    <IconButton
      key={action.id}
      data-reveal="hover"
      icon={iconGlyph(knownIconName(action.icon) ?? "tool")}
      size="xs"
      quiet
      aria-label={t(action.title)}
      aria-describedby={summaryId}
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
  ));
  if (opener) {
    controls.push(
      <IconButton
        key="open-view"
        data-reveal="hover"
        data-slot="tool-open-view"
        icon={iconGlyph("open")}
        size="xs"
        quiet
        aria-label={t(opener.label(tool))}
        aria-describedby={summaryId}
        onClick={(event) => {
          event.stopPropagation();
          void Promise.resolve(opener.open(tool, event.currentTarget)).catch((err) => {
            const owner = lookupToolViewOpenerOwner(opener.id) ?? "unknown";
            console.error(`[plugin] tool view opener ${opener.id} threw:`, err);
            reportPluginError(owner, "command", err, `tool view opener: ${opener.id}`);
          });
        }}
        className={stylex.props(reveal.shown).className}
      />,
    );
  }
  return controls;
}

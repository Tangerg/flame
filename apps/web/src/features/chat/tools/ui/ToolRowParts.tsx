import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { type ToolCall } from "@/features/agent/public/sessionView";
import { knownIconName } from "@/ui/index";
import { reveal } from "@/ui/index";
import { StatusDot } from "@/ui/index";
import { IconButton, vocab } from "@flame/ui";
import { useMemo } from "react";
import { useT } from "@/lib/i18n/index";
import { type AsyncFeedback, useAsyncFeedback } from "@/lib/useAsyncFeedback";
import { failureText } from "@/lib/rpcErrors";
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
  preWrap: { whiteSpace: "pre-wrap" },
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

export function ToolActionFailureLine({
  feedback,
  summaryId,
}: {
  feedback: AsyncFeedback;
  summaryId: string;
}) {
  if (feedback.state !== "error") return null;
  return (
    <p
      data-slot="tool-action-error"
      role="alert"
      aria-describedby={summaryId}
      {...stylex.props(styles.failureLine, styles.preWrap, typeStep.uiSm)}
    >
      {feedback.reason}
    </p>
  );
}

export function useToolRowActions(tool: ToolCall, summaryId: string) {
  const t = useT();
  const actions = toolCardActions(tool, useExtensionPoint(TOOL_ACTION));
  const opener = toolCardViewOpener(tool, useExtensionPoint(TOOL_VIEW_OPENER));
  // Keyed to the call's identity, not the snapshot object: a refreshed snapshot
  // of the same call keeps its outcome, a different call retires it.
  const identity = useMemo(() => ({ runId: tool.runId, id: tool.id }), [tool.runId, tool.id]);
  const { feedback, run } = useAsyncFeedback(identity);

  // The callback is invoked synchronously inside the activation, so a sync
  // throw and a rejection take the same path. The owner is read before the
  // call so a later failure stays attributed to the contributor that ran.
  const activate = (
    label: string,
    owner: string,
    log: string,
    detail: string,
    invoke: () => void | Promise<void>,
  ) => {
    void run(async () => {
      try {
        await invoke();
        return { ok: true };
      } catch (err) {
        console.error(log, err);
        reportPluginError(owner, "command", err, detail);
        return {
          ok: false,
          error: `${t("toolAction.failed", { action: label })} ${failureText(err, String(err))}`,
        };
      }
    }, label);
  };

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
        activate(
          t(action.title),
          lookupToolActionOwner(action.id) ?? "unknown",
          `[plugin] tool action ${action.id} threw:`,
          `tool action: ${action.id}`,
          () => action.run(tool),
        );
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
          const anchor = event.currentTarget;
          activate(
            t(opener.label(tool)),
            lookupToolViewOpenerOwner(opener.id) ?? "unknown",
            `[plugin] tool view opener ${opener.id} threw:`,
            `tool view opener: ${opener.id}`,
            () => opener.open(tool, anchor),
          );
        }}
        className={stylex.props(reveal.shown).className}
      />,
    );
  }
  return { controls, feedback };
}

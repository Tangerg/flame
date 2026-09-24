import * as stylex from "@stylexjs/stylex";
import type { AgentRunView } from "@/plugins/sdk/types/agentSessionView";
import { Icon, IconButton, Pressable, reveal, StatusDot, toneInk, vocab } from "@/ui";
import { useT } from "@/lib/i18n";
import { cancelSessionRun } from "@/plugins/builtin/agent/public/run";
import { openWorkspaceSubagentRun } from "@/plugins/builtin/workspace/public/navigation";
import { useRuntimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";
import { motion, radius, space, surface, type as typeStep } from "@/styles/tokens.stylex";
import { delegatedRunSummary } from "../application/delegatedRunSummary";

const styles = stylex.create({
  row: {
    display: "flex",
    alignItems: "center",
    gap: space.s1,
    minWidth: 0,
    marginInlineStart: "calc(var(--spacing) * -2)",
  },
  openHint: {
    transitionProperty: "opacity",
    transitionDuration: motion.fast,
    transitionTimingFunction: motion.easeState,
  },
  link: {
    display: "flex",
    alignItems: "flex-start",
    gap: space.s2,
    minWidth: 0,
    flex: 1,
    textAlign: "start",
    paddingBlock: space.s2,
    paddingInline: space.s2,
    borderRadius: radius.row,
    backgroundColor: { default: null, ":hover": surface.hover },
    transitionProperty: "background-color",
    transitionDuration: motion.color,
    transitionTimingFunction: motion.easeState,
  },
  detail: { display: "flex", flexDirection: "column", gap: space.s0_5, minWidth: 0, flex: 1 },
  name: { minWidth: 0, flex: 1 },
  cancelSlot: {
    display: "flex",
    flexShrink: 0,
    justifyContent: "center",
    width: "var(--control-height-sm)",
  },
});

export function DelegatedRunLink({
  run,
  ordinal,
  siblingCount,
  taskLabel,
}: {
  run: AgentRunView;
  ordinal: number;
  siblingCount: number;
  taskLabel?: string;
}) {
  const t = useT();
  const available = useRuntimeCommandsAvailable();
  const model = delegatedRunSummary(t, run, ordinal, siblingCount, taskLabel);
  return (
    <div
      data-slot="delegated-run-link"
      data-run-id={run.id}
      {...stylex.props(styles.row, reveal.host)}
    >
      <Pressable
        onClick={() => openWorkspaceSubagentRun(run.id)}
        className={stylex.props(styles.link).className}
      >
        <span {...stylex.props(vocab.firstLine, typeStep.uiMd)}>
          <Icon name="bot" size="md" />
        </span>
        <span {...stylex.props(styles.detail)}>
          <span {...stylex.props(vocab.line, typeStep.uiMd)}>
            <span title={model.label} {...stylex.props(styles.name, vocab.truncate)}>
              {model.label}
            </span>
            <StatusDot tone={model.dotTone} />
            <span {...stylex.props(vocab.hold, toneInk[model.ink])}>{model.statusLabel}</span>
          </span>
          {model.detail && (
            <span {...stylex.props(vocab.muted, vocab.truncate, typeStep.uiXs)}>
              {model.detail}
            </span>
          )}
        </span>
        <span
          data-reveal="hover"
          {...stylex.props(vocab.firstLine, typeStep.uiMd, reveal.shown, styles.openHint)}
        >
          <Icon name="open" size="sm" />
        </span>
      </Pressable>
      <span {...stylex.props(styles.cancelSlot)}>
        {model.cancelable && (
          <IconButton
            icon="stop"
            size="sm"
            quiet
            disabled={!available}
            title={t("agent.runTree.action.cancel")}
            onClick={() => cancelSessionRun({ sessionId: run.sessionId, runId: run.id })}
          />
        )}
      </span>
    </div>
  );
}

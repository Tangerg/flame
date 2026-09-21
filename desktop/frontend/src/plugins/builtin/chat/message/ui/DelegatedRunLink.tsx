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
  row: { display: "flex", alignItems: "center", gap: space.s1, minWidth: 0 },
  // Four identical open-in-panel glyphs stacked into a column of their own beside four rows
  // that differ only in their status. The row already answers the pointer; the glyph says
  // where the click lands, which is worth saying at the moment the pointer is there.
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
    // The row is the view's main affordance and answered the pointer with nothing — measured,
    // its background stayed transparent on hover while every other dock list washed at 5%.
    // A row that opens something has to look like it does.
    backgroundColor: { default: null, ":hover": surface.hover },
    transitionProperty: "background-color",
    transitionDuration: motion.color,
    transitionTimingFunction: motion.easeState,
  },
  detail: { display: "flex", flexDirection: "column", gap: space.s0_5, minWidth: 0, flex: 1 },
  // The NAME takes the slack, so the outcome lands on the row's end rather than wherever the
  // name happened to stop. Four rows of a fan-out reported their status at four different
  // offsets, which is the column a reader scans to find the one that needs them.
  name: { minWidth: 0, flex: 1 },
  // Held open whether or not the run can be cancelled. The button used to appear only on the
  // rows that could, so those rows ended 30px short of the ones that could not and the
  // outcome column zig-zagged down a fan-out.
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
        <span {...stylex.props(vocab.firstLine, typeStep.uiSm)}>
          <Icon name="bot" size="sm" />
        </span>
        <span {...stylex.props(styles.detail)}>
          <span {...stylex.props(vocab.line, typeStep.uiSm)}>
            <span title={model.label} {...stylex.props(styles.name, vocab.truncate)}>
              {model.label}
            </span>
            <StatusDot tone={model.dotTone} />
            <span {...stylex.props(toneInk[model.ink])}>{model.statusLabel}</span>
          </span>
          {model.detail && (
            <span {...stylex.props(vocab.muted, vocab.truncate, typeStep.uiXs)}>
              {model.detail}
            </span>
          )}
        </span>
        <span
          data-reveal="hover"
          {...stylex.props(vocab.firstLine, typeStep.uiSm, reveal.shown, styles.openHint)}
        >
          <Icon name="panel-r" size="sm" />
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

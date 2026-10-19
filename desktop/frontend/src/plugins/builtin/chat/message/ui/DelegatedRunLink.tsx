import * as stylex from "@stylexjs/stylex";
import type { AgentRunView } from "@/plugins/sdk/types/agentSessionView";
import { Icon, Pressable, reveal, StatusDot } from "@/ui";
import { RecordSummary, toneInk, vocab } from "@flame/ui";
import { useT } from "@/lib/i18n";
import { openWorkspaceSubagentRun } from "@/plugins/builtin/workspace/public/navigation";
import { motion, radius, space, surface, type as typeStep } from "@flame/ui/tokens.stylex";
import { delegatedRunSummary } from "../application/delegatedRunSummary";
import { SubagentStopButton } from "./SubagentStopButton";

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
  cancel: { alignSelf: "flex-start", marginBlockStart: space.s2 },
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
          <RecordSummary
            title={<span title={model.label}>{model.label}</span>}
            status={
              <span {...stylex.props(vocab.line, vocab.hold, toneInk[model.ink])}>
                <StatusDot tone={model.dotTone} />
                {model.statusLabel}
              </span>
            }
          />
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
      {model.cancelable && (
        <span {...stylex.props(styles.cancel, vocab.firstLine, typeStep.uiMd)}>
          <SubagentStopButton run={run} />
        </span>
      )}
    </div>
  );
}

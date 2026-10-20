import * as stylex from "@stylexjs/stylex";
import { type AgentRunView } from "@/features/agent/public/sessionView";
import { Icon } from "@/ui/index";
import { Pressable } from "@/ui/index";
import { reveal } from "@/ui/index";
import { StatusDot } from "@/ui/index";
import { RecordSummary, toneInk, vocab } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import {
  openWorkspaceSubagentRun,
  useWorkspaceSubagentRunId,
} from "@/features/workspace/public/navigation";
import { motion, radius, space, surface, type as typeStep } from "@flame/ui/tokens.stylex";
import { delegatedRunSummary } from "@/features/chat/message/application/delegatedRunSummary";
import { SubagentStopButton } from "@/features/chat/message/ui/SubagentStopButton";
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
  currentHint: { opacity: 1 },
  current: { backgroundColor: { default: surface.selected, ":hover": surface.selected } },
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
  onOpen,
}: {
  run: AgentRunView;
  ordinal: number;
  siblingCount: number;
  taskLabel?: string;
  onOpen?: () => void;
}) {
  const t = useT();
  const current = useWorkspaceSubagentRunId() === run.id;
  const model = delegatedRunSummary(t, run, ordinal, siblingCount, taskLabel);
  return (
    <div
      data-slot="delegated-run-link"
      data-run-id={run.id}
      {...stylex.props(styles.row, reveal.host)}
    >
      <Pressable
        aria-current={current ? "true" : undefined}
        onClick={(event) =>
          onOpen ? onOpen() : openWorkspaceSubagentRun(run.id, event.currentTarget)
        }
        className={stylex.props(styles.link, current && styles.current).className}
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
          data-reveal={current ? undefined : "hover"}
          {...stylex.props(
            vocab.firstLine,
            typeStep.uiMd,
            reveal.shown,
            styles.openHint,
            current && styles.currentHint,
          )}
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

import * as stylex from "@stylexjs/stylex";
import { useT } from "@/lib/i18n";
import { color, radius, space, type as typeStep, weight } from "@/styles/tokens.stylex";
import { Icon, IconButton, Popover, ProgressBar, SectionLabel, toneInk, vocab } from "@/ui";
import type { TaskReadoutStatus, TaskReadoutTask } from "../application/ports/taskReadoutPort";
import { taskProgressPercent, useTaskReadout } from "../application/taskReadout";

// The icon reports the status, and it inherits the control's ink — so the tone is the button's
// rather than a class on the glyph. `running` has no tone: it is the ordinary state, and the
// chrome's own ink is what ordinary looks like.
const STATUS_ICON: Record<
  TaskReadoutStatus,
  { name: "spark" | "check" | "x"; tone?: "accent" | "negative" }
> = {
  running: { name: "spark" },
  succeeded: { name: "check", tone: "accent" },
  failed: { name: "x", tone: "negative" },
};

// The task list's glyph column is 18px wide — the icon plus its gap — so a message under a
// label starts where the label does rather than under the glyph.
const GLYPH_COLUMN = "18px";

const p = stylex.create({
  panel: { width: "calc(var(--spacing) * 80)", borderRadius: radius.xl },
  header: { paddingInline: space.s3, paddingTop: space.s2, paddingBottom: space.s1 },
  scroller: { maxHeight: "min(280px, var(--available-height))", overflowY: "auto" },
  row: { paddingInline: space.s3, paddingBlock: space.s2 },
  label: {
    flex: 1,
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
    color: color.fg,
    fontWeight: weight.semibold,
  },
  percent: { fontFamily: "var(--font-mono)", color: color.fgFaint },
  detail: { marginTop: space.s0_5, paddingLeft: GLYPH_COLUMN, color: color.fgMuted },
  error: { color: color.negative },
  meter: { marginTop: space.s1_5, marginLeft: GLYPH_COLUMN },
  pulse: { animation: "var(--animate-pulse-dot)" },
});

export function TasksPill() {
  const readout = useTaskReadout();
  const t = useT();
  if (!readout) return null;

  const { name, tone } = STATUS_ICON[readout.head.status];

  return (
    <Popover.Root>
      <Popover.Trigger
        render={
          <IconButton
            icon={name}
            size="sm"
            tone={tone}
            badge={readout.runningCount}
            aria-label={readout.label}
            data-pulse={readout.head.status === "running" ? "" : undefined}
          />
        }
      />
      <Popover.Content
        side="top"
        align="start"
        sideOffset={6}
        className={stylex.props(p.panel).className}
      >
        <SectionLabel className={stylex.props(p.header).className}>
          {t("tasks.header")}
        </SectionLabel>
        <div {...stylex.props(p.scroller)}>
          {readout.tasks.map((task) => (
            <TaskRow key={task.id} task={task} />
          ))}
        </div>
      </Popover.Content>
    </Popover.Root>
  );
}

function TaskRow({ task }: { task: TaskReadoutTask }) {
  const { name, tone } = STATUS_ICON[task.status];
  const percent = taskProgressPercent(task);

  return (
    <div {...stylex.props(p.row)}>
      <div {...stylex.props(vocab.line)}>
        <Icon
          name={name}
          size="xs"
          className={
            stylex.props(
              tone === undefined ? vocab.ink : toneInk[tone],
              task.status === "running" && p.pulse,
            ).className
          }
        />
        <span {...stylex.props(p.label, typeStep.uiMd)}>{task.label}</span>
        {percent !== null && <span {...stylex.props(p.percent, typeStep.uiSm)}>{percent}%</span>}
      </div>
      {task.message && <div {...stylex.props(p.detail, typeStep.uiSm)}>{task.message}</div>}
      {task.error && <div {...stylex.props(p.detail, p.error, typeStep.uiSm)}>{task.error}</div>}
      {percent !== null && (
        <ProgressBar
          value={percent}
          label={task.label}
          className={stylex.props(p.meter).className}
        />
      )}
    </div>
  );
}

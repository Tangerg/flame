import { useMemo } from "react";
import { useTasksStore } from "@/extensions/tasksStore";
import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { useT } from "@/lib/i18n/index";
import { color, motion, radius, space, type as typeStep, weight } from "@flame/ui/tokens.stylex";
import { Icon, type IconName } from "@/ui/index";
import { Popover } from "@/ui/index";
import { ProgressBar } from "@/ui/index";
import { SectionLabel } from "@/ui/index";
import { IconButton, ScrollRegion, toneInk, vocab } from "@flame/ui";
import { type TaskEntry, type TaskStatus } from "@/extensions/tasksStore";
import { taskProgressPercent, taskReadout } from "@/features/workspace/application/taskReadout";
const STATUS_ICON: Record<
  TaskStatus,
  { name: IconName; label: string; tone?: "accent" | "negative" }
> = {
  running: { name: "activity", label: "tasks.status.running" },
  succeeded: { name: "circle-check", label: "tasks.status.succeeded", tone: "accent" },
  failed: { name: "circle-x", label: "tasks.status.failed", tone: "negative" },
};

const p = stylex.create({
  panel: {
    display: "flex",
    flexDirection: "column",
    width: "calc(var(--spacing) * 80)",
    maxWidth: "var(--available-width)",
    maxHeight: "var(--available-height)",
    borderRadius: radius.xl,
  },
  header: { flexShrink: 0, paddingInline: space.s3, paddingTop: space.s2, paddingBottom: space.s1 },
  scroller: { minHeight: 0, maxHeight: "280px", paddingBlock: 0 },
  row: {
    display: "grid",
    gridTemplateColumns: "auto minmax(0, 1fr)",
    columnGap: space.s1_5,
    paddingInline: space.s3,
    paddingBlock: space.s2,
  },
  mark: { display: "inline-flex", marginTop: space.s1 },
  body: { minWidth: 0 },
  line: { alignItems: "flex-start" },
  label: {
    flex: 1,
    minWidth: 0,
    overflowWrap: "anywhere",
    color: color.fg,
    fontWeight: weight.semibold,
  },
  percent: {
    flexShrink: 0,
    whiteSpace: "nowrap",
    fontFamily: "var(--font-mono)",
    color: color.fgFaint,
  },
  detail: {
    marginTop: space.s0_5,
    whiteSpace: "pre-wrap",
    overflowWrap: "anywhere",
    color: color.fgMuted,
  },
  error: { color: color.negative },
  meter: { marginTop: space.s1_5 },
  pulse: { animation: motion.pulseDot },
});

export function TasksPill() {
  const t = useT();
  const tasks = useTasksStore((state) => state.tasks);
  const readout = useMemo(() => taskReadout(t, tasks), [t, tasks]);
  if (!readout) return null;

  const { name, tone } = STATUS_ICON[readout.head.status];

  return (
    <Popover.Root>
      <Popover.Trigger
        render={
          <IconButton
            icon={iconGlyph(name)}
            size="sm"
            tone={tone}
            badge={readout.runningCount}
            aria-label={readout.label}
            data-pulse={readout.head.status === "running" ? "" : undefined}
          />
        }
      />
      <Popover.Content
        aria-label={t("tasks.header")}
        side="top"
        align="start"
        sideOffset={6}
        className={stylex.props(p.panel).className}
      >
        <SectionLabel className={stylex.props(p.header).className}>
          {t("tasks.header")}
        </SectionLabel>
        <ScrollRegion label={t("tasks.header")} styles={p.scroller}>
          {readout.tasks.map((task) => (
            <TaskRow key={task.id} task={task} />
          ))}
        </ScrollRegion>
      </Popover.Content>
    </Popover.Root>
  );
}

function TaskRow({ task }: { task: TaskEntry }) {
  const t = useT();
  const { name, label, tone } = STATUS_ICON[task.status];
  const percent = taskProgressPercent(task);

  return (
    <div {...stylex.props(p.row)}>
      <span role="img" aria-label={t(label)} {...stylex.props(p.mark)}>
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
      </span>
      <div {...stylex.props(p.body)}>
        <div {...stylex.props(vocab.line, p.line)}>
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
    </div>
  );
}

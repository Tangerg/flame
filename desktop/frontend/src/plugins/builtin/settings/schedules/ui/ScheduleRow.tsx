import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useState } from "react";
import { ConfirmDialog, IconButton, Switch, Tag, type IconName, vocab } from "@/ui";
import {
  deleteSchedule,
  runScheduleNow,
  setScheduleEnabled,
  type ScheduleConfig,
} from "../application/scheduleCommands";
import { useCommandAction } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { formatDateTime } from "@/lib/i18n/relativeTime";
import { ScheduleForm } from "./ScheduleForm";
import {
  color,
  face,
  leading,
  radius,
  space,
  surface,
  type as typeStep,
  weight,
} from "@/styles/tokens.stylex";

function ScheduleActionButton({
  icon,
  label,
  title,
  active,
  tone,
  busy,
  onClick,
}: {
  icon: IconName;
  label: string;
  title?: string;
  active?: boolean;
  tone?: "accent" | "negative";
  busy?: boolean;
  onClick: () => void;
}) {
  return (
    <IconButton
      icon={icon}
      iconSize="sm"
      size="sm"
      quiet
      aria-label={label}
      aria-expanded={active}
      aria-busy={busy}
      pending={busy}
      title={title}
      onClick={onClick}
      tone={tone}
    />
  );
}

const sr = stylex.create({
  row: {
    display: "grid",
    gridTemplateColumns: "minmax(0, 1fr) auto",
    alignItems: "flex-start",
    gap: space.s3,
    borderRadius: radius.card,
    backgroundColor: { default: null, ":hover": surface.hover },
    paddingInline: space.s3,
    paddingBlock: space.s2_5,
    transitionProperty: "background-color",
  },
  title: { fontWeight: weight.medium },
  titleOn: { color: color.fg },
  cron: { marginTop: space.s0_5, color: color.fgMuted, lineHeight: leading.body },
  stamps: {
    marginTop: space.s1,
    display: "flex",
    flexWrap: "wrap",
    columnGap: space.s3,
    color: color.fgFaint,
  },
  editor: { marginTop: space.s2_5 },
});

export function ScheduleRow({ schedule }: { schedule: ScheduleConfig }) {
  const t = useT();
  const [editing, setEditing] = useState(false);
  const [confirmingDelete, setConfirmingDelete] = useState(false);

  const { busy, run } = useCommandAction({
    wasRetired: wasGenerationRetired,
    fallback: t("schedules.error.save"),
  });

  return (
    <div>
      <div {...stylex.props(sr.row)}>
        {/* Only what the row SAYS steps back, never what it offers. `opacity-60` on the whole
            row put its run, edit and delete controls BELOW the opacity the app draws a
            genuinely disabled control at, and measured on the rendered pixels it took the
            title to 4.36:1, the cron to 2.43 and the instructions to 2.68 — a row marked
            inactive by making itself unreadable. A step down the token ladder is the same
            signal with contrast the design system owns rather than a multiplier landing
            wherever the two colours happen to leave it. */}
        <div {...stylex.props(vocab.min)}>
          <div {...stylex.props(vocab.line)}>
            <span
              {...stylex.props(
                vocab.truncate,
                sr.title,
                schedule.enabled ? sr.titleOn : vocab.muted,
                typeStep.uiMd,
              )}
            >
              {schedule.title || t("schedules.untitled")}
            </span>
            <Tag size="sm">{schedule.cron}</Tag>
          </div>
          <div
            {...stylex.props(sr.cron, vocab.truncate, typeStep.uiMd, face.mono)}
            title={schedule.instructions}
          >
            {schedule.instructions}
          </div>
          <div {...stylex.props(sr.stamps, typeStep.uiSm)}>
            {schedule.enabled && schedule.nextRunAt && (
              <span>{t("schedules.next", { time: formatDateTime(schedule.nextRunAt) })}</span>
            )}
            {schedule.lastRunAt && (
              <span>{t("schedules.last", { time: formatDateTime(schedule.lastRunAt) })}</span>
            )}
          </div>
        </div>
        <div {...stylex.props(vocab.lineTight)}>
          <Switch
            checked={schedule.enabled}
            disabled={busy}
            onCheckedChange={(value) => run(() => setScheduleEnabled(schedule, value))}
            ariaLabel={t("schedules.enable.aria")}
          />
          <ScheduleActionButton
            icon="play"
            label={t("schedules.runNow")}
            title={t("schedules.runNow")}
            tone="accent"
            busy={busy}
            onClick={() => run(() => runScheduleNow(schedule.id))}
          />
          <ScheduleActionButton
            icon="edit"
            label={t("schedules.edit")}
            active={editing}
            onClick={() => setEditing((value) => !value)}
          />
          <ScheduleActionButton
            icon="trash"
            label={t("schedules.delete")}
            title={t("schedules.delete")}
            tone="negative"
            busy={busy}
            onClick={() => setConfirmingDelete(true)}
          />
        </div>
      </div>

      {editing && (
        <div {...stylex.props(sr.editor)}>
          <ScheduleForm
            schedule={schedule}
            onDone={() => setEditing(false)}
            onCancel={() => setEditing(false)}
          />
        </div>
      )}

      {/* The least-protected destructive action in the app until now: one click on a quiet
          icon between Run and Edit, no menu in front of it, no undo behind it. A session's
          delete — the app's other row-level one — asks first, and it is already behind a
          context menu. This is a saved schedule and its instructions, gone on a slip. */}
      <ConfirmDialog
        open={confirmingDelete}
        onOpenChange={setConfirmingDelete}
        title={t("schedules.delete.title")}
        body={t("schedules.delete.body", { title: schedule.title || t("schedules.untitled") })}
        confirmLabel={t("schedules.delete.confirm")}
        cancelLabel={t("common.cancel")}
        destructive
        onConfirm={() => run(() => deleteSchedule(schedule.id))}
      />
    </div>
  );
}

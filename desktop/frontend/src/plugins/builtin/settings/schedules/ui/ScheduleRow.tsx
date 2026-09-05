import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useState } from "react";
import { ConfirmDialog, IconButton, Switch, Tag, type IconName } from "@/ui";
import {
  deleteSchedule,
  runScheduleNow,
  setScheduleEnabled,
  type ScheduleConfig,
} from "../application/scheduleCommands";
import { useCommandAction } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { formatDateTime } from "@/lib/i18n/relativeTime";
import { cn } from "@/lib/classNames";
import { ScheduleForm } from "./ScheduleForm";

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
      disabled={busy}
      title={title}
      onClick={onClick}
      className={cn(
        tone === "accent" && "hover:text-accent",
        tone === "negative" && "hover:text-negative",
      )}
    />
  );
}

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
      <div className="grid grid-cols-[minmax(0,1fr)_auto] items-start gap-3 rounded-md px-3 py-2.5 transition-colors hover:bg-hover">
        {/* Only what the row SAYS steps back, never what it offers. `opacity-60` on the whole
            row put its run, edit and delete controls BELOW the opacity the app draws a
            genuinely disabled control at, and measured on the rendered pixels it took the
            title to 4.36:1, the cron to 2.43 and the instructions to 2.68 — a row marked
            inactive by making itself unreadable. A step down the token ladder is the same
            signal with contrast the design system owns rather than a multiplier landing
            wherever the two colours happen to leave it. */}
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span
              className={cn(
                "truncate text-ui-md font-medium",
                schedule.enabled ? "text-fg" : "text-fg-muted",
              )}
            >
              {schedule.title || t("schedules.untitled")}
            </span>
            <Tag size="sm">{schedule.cron}</Tag>
          </div>
          <div
            className="mt-0.5 truncate font-mono text-ui-md leading-body text-fg-muted"
            title={schedule.instructions}
          >
            {schedule.instructions}
          </div>
          <div className="mt-1 flex flex-wrap gap-x-3 text-ui-sm text-fg-faint">
            {schedule.enabled && schedule.nextRunAt && (
              <span>{t("schedules.next", { time: formatDateTime(schedule.nextRunAt) })}</span>
            )}
            {schedule.lastRunAt && (
              <span>{t("schedules.last", { time: formatDateTime(schedule.lastRunAt) })}</span>
            )}
          </div>
        </div>
        <div className="flex items-center gap-1.5">
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
        <div className="mt-2.5">
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

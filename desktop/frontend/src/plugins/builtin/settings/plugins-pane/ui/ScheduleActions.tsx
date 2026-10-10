import { useState } from "react";
import * as stylex from "@stylexjs/stylex";
import type { PluginScheduleTemplate } from "@flame/runtime-contract/wire";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import {
  ScheduleForm,
  useSchedules,
  setScheduleEnabled,
  deleteSchedule,
  runScheduleNow,
  type ScheduleConfig,
} from "@/plugins/builtin/settings/schedules/public/controls";
import { ConfirmDialog, DropdownMenu, PillButton, SelectTrigger, SystemMessage, vocab } from "@/ui";
import { useT } from "@/lib/i18n";
import { space } from "@/styles/tokens.stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useAsyncFeedback } from "../../kit";

const styles = stylex.create({
  root: { padding: space.s3, flexShrink: 0, maxHeight: "60%", overflow: "auto" },
  controls: { display: "flex", flexWrap: "wrap", gap: space.s2 },
  editor: { paddingTop: space.s2 },
  picker: { maxWidth: "100%" },
});
type Edit =
  | { type: "add"; cwd?: string; template?: PluginScheduleTemplate }
  | { type: "update"; schedule: ScheduleConfig };

export function ScheduleActions({
  templates,
  signal,
  onSaved,
}: {
  templates: PluginScheduleTemplate[];
  signal: AbortSignal;
  onSaved(): void;
}) {
  const t = useT();
  const workspace = useActiveSessionWorkspace();
  const { data: schedules = [], isLoading, error, refetch } = useSchedules();
  const [selectedId, setSelectedId] = useState<string>();
  const selected = schedules.find((schedule) => schedule.id === selectedId);
  const [edit, setEdit] = useState<Edit>();
  const [deleting, setDeleting] = useState<ScheduleConfig>();
  const { feedback, run } = useAsyncFeedback(signal);
  const busy = feedback.state === "busy";
  const saved = () => {
    if (signal.aborted) return;
    setEdit(undefined);
    onSaved();
  };
  const add = (template?: PluginScheduleTemplate) => {
    setEdit({
      type: "add",
      cwd: workspace.status === "ready" ? workspace.cwd : undefined,
      template,
    });
  };
  const execute = (operation: () => Promise<unknown>) =>
    void run(
      async () => {
        signal.throwIfAborted();
        await operation();
        saved();
        return { ok: true };
      },
      t("schedules.error.save"),
      wasGenerationRetired,
    );
  return (
    <div {...stylex.props(styles.root)}>
      <div {...stylex.props(styles.controls)}>
        <PillButton
          size="sm"
          pending={busy}
          disabled={workspace.status === "resolving"}
          onClick={() => add()}
        >
          {t("schedules.add")}
        </PillButton>
        {templates.length > 0 && (
          <DropdownMenu.Root>
            <DropdownMenu.Trigger
              render={
                <SelectTrigger
                  aria-label={t("schedules.template")}
                  label={t("schedules.template")}
                  pending={busy}
                  disabled={workspace.status === "resolving"}
                />
              }
            />
            <DropdownMenu.Content align="start">
              {templates.map((template) => (
                <DropdownMenu.Item
                  key={template.id}
                  layout="pickPlain"
                  onClick={() => add(template)}
                >
                  {template.title} · {template.cron}
                </DropdownMenu.Item>
              ))}
            </DropdownMenu.Content>
          </DropdownMenu.Root>
        )}
        <DropdownMenu.Root>
          <DropdownMenu.Trigger
            render={
              <SelectTrigger
                aria-label={t("schedules.select")}
                label={
                  selected
                    ? `${selected.id} · ${selected.title || t("schedules.untitled")}`
                    : t("schedules.select")
                }
                pending={busy}
                disabled={isLoading || Boolean(error) || schedules.length === 0}
                className={stylex.props(styles.picker).className}
              />
            }
          />
          <DropdownMenu.Content align="start">
            {schedules.map((schedule) => (
              <DropdownMenu.Item
                key={schedule.id}
                layout="pickPlain"
                onClick={() => {
                  setSelectedId(schedule.id);
                  setEdit(undefined);
                }}
              >
                <span {...stylex.props(vocab.truncate)}>
                  {schedule.id} · {schedule.title || t("schedules.untitled")}
                </span>
              </DropdownMenu.Item>
            ))}
          </DropdownMenu.Content>
        </DropdownMenu.Root>
        {selected && !error && !isLoading && (
          <>
            <PillButton
              size="sm"
              pending={busy}
              onClick={() => execute(() => setScheduleEnabled(selected, !selected.enabled))}
            >
              {t(selected.enabled ? "schedules.disable" : "schedules.enable")}
            </PillButton>
            <PillButton
              size="sm"
              pending={busy}
              onClick={() => execute(() => runScheduleNow(selected.id))}
            >
              {t("schedules.runNow")}
            </PillButton>
            <PillButton
              size="sm"
              pending={busy}
              onClick={() => setEdit({ type: "update", schedule: selected })}
            >
              {t("schedules.edit")}
            </PillButton>
            <PillButton
              size="sm"
              pending={busy}
              variant="danger"
              onClick={() => setDeleting(selected)}
            >
              {t("schedules.delete")}
            </PillButton>
          </>
        )}
      </div>
      {isLoading && <SystemMessage>{t("packages.view.loading")}</SystemMessage>}
      {error && (
        <SystemMessage variant="error">
          {error.message}
          <PillButton size="sm" onClick={() => void refetch()}>
            {t("common.retry")}
          </PillButton>
        </SystemMessage>
      )}
      {feedback.state === "error" && (
        <SystemMessage variant="error">{feedback.reason}</SystemMessage>
      )}
      {edit && (
        <div {...stylex.props(styles.editor)}>
          <ScheduleForm
            signal={signal}
            key={
              edit.type === "update"
                ? `${edit.schedule.id}:${edit.schedule.revision}`
                : (edit.template?.id ?? "new")
            }
            schedule={edit.type === "update" ? edit.schedule : undefined}
            defaultCwd={edit.type === "add" ? edit.cwd : undefined}
            template={edit.type === "add" ? edit.template : undefined}
            onDone={saved}
            onCancel={() => setEdit(undefined)}
          />
        </div>
      )}
      <ConfirmDialog
        open={Boolean(deleting)}
        onOpenChange={(open) => {
          if (!open) setDeleting(undefined);
        }}
        title={t("schedules.delete.title")}
        body={t("schedules.delete.body", { title: deleting?.title || t("schedules.untitled") })}
        confirmLabel={t("schedules.delete.confirm")}
        cancelLabel={t("common.cancel")}
        destructive
        onConfirm={() => {
          if (deleting) execute(() => deleteSchedule(deleting.id));
        }}
      />
    </div>
  );
}

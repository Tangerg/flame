import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useState } from "react";
import { PillButton, Surface, TextArea, TextField, vocab } from "@/ui";
import {
  createSchedule,
  updateSchedule,
  type ScheduleConfig,
} from "../application/scheduleCommands";
import { useCommandAction } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import {
  type ScheduleDraft,
  canSaveScheduleDraft,
  initialScheduleDraft,
  scheduleInputFromDraft,
} from "../application/scheduleDraft";
import { settingStyles as ss } from "../../kit/settingStyles";
import { ScheduleModelFields } from "./ScheduleModelFields";

interface ScheduleFormProps {
  signal: AbortSignal;
  schedule?: ScheduleConfig;
  defaultCwd?: string;
  template?: Pick<ScheduleConfig, "title" | "instructions" | "cron">;
  onDone: () => void;
  onCancel: () => void;
}

export function ScheduleForm({
  schedule,
  defaultCwd,
  template,
  signal,
  onDone,
  onCancel,
}: ScheduleFormProps) {
  const t = useT();
  const [basis] = useState(() =>
    schedule ? { id: schedule.id, revision: schedule.revision } : undefined,
  );
  const [original] = useState<ScheduleDraft>(() =>
    initialScheduleDraft(schedule, defaultCwd, template),
  );
  const [draft, setDraft] = useState(original);
  const { busy, run } = useCommandAction({
    wasRetired: (error) =>
      wasGenerationRetired(error) || (signal.aborted && error === signal.reason),
    fallback: t("schedules.error.save"),
  });

  const updateDraft = <K extends keyof ScheduleDraft>(key: K, value: ScheduleDraft[K]) => {
    setDraft((current) => ({ ...current, [key]: value }));
  };

  const onSave = () =>
    run(async () => {
      signal.throwIfAborted();
      const input = scheduleInputFromDraft(draft, original);
      if (basis) {
        await updateSchedule({
          ...input,
          ...basis,
        });
      } else {
        await createSchedule(input);
      }
      if (!signal.aborted) onDone();
    });

  return (
    <Surface className={stylex.props(ss.stack).className}>
      <TextField
        value={draft.title}
        onChange={(event) => updateDraft("title", event.target.value)}
        placeholder={t("schedules.form.title")}
        aria-label={t("schedules.form.title")}
      />
      <TextArea
        size="sm"
        value={draft.instructions}
        onChange={(event) => updateDraft("instructions", event.target.value)}
        rows={4}
        placeholder={t("schedules.form.instructions")}
        aria-label={t("schedules.form.instructions")}
      />
      <TextField
        font="mono"
        value={draft.cron}
        onChange={(event) => updateDraft("cron", event.target.value)}
        spellCheck={false}
        placeholder={t("schedules.form.cron")}
        aria-label={t("schedules.form.cron")}
      />
      <TextField
        font="mono"
        value={draft.cwd}
        onChange={(event) => updateDraft("cwd", event.target.value)}
        spellCheck={false}
        placeholder={t("schedules.form.cwd")}
        aria-label={t("schedules.form.cwd")}
      />
      <div {...stylex.props(vocab.line)}>
        <ScheduleModelFields
          selection={draft.modelSelection}
          onChange={(selection) => updateDraft("modelSelection", selection)}
        />
      </div>
      <div {...stylex.props(vocab.line)}>
        <PillButton
          variant="solid"
          size="sm"
          disabled={!canSaveScheduleDraft(draft)}
          pending={busy}
          onClick={onSave}
        >
          {busy ? t("schedules.saving") : t("schedules.save")}
        </PillButton>
        <PillButton variant="outlined" size="sm" onClick={onCancel}>
          {t("common.cancel")}
        </PillButton>
      </div>
    </Surface>
  );
}

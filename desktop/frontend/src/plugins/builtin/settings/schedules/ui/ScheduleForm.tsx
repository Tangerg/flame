import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useState } from "react";
import { PillButton, Pressable, Surface, TextArea, TextField, vocab } from "@/ui";
import {
  createSchedule,
  updateSchedule,
  type ScheduleConfig,
} from "../application/scheduleCommands";
import { useCommandAction } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import {
  CRON_PRESETS,
  type ScheduleDraft,
  canSaveScheduleDraft,
  initialScheduleDraft,
  scheduleInputFromDraft,
} from "../application/scheduleDraft";
import { color, corner, space, surface, type as typeStep, weight } from "@/styles/tokens.stylex";
import { settingStyles as ss } from "../../kit/settingStyles";

interface ScheduleFormProps {
  schedule?: ScheduleConfig;
  defaultCwd?: string;
  onDone: () => void;
  onCancel: () => void;
}

const sf = stylex.create({
  preset: {
    borderWidth: "1px",
    borderStyle: "solid",
    paddingInline: space.s2_5,
    paddingBlock: space.s1,
    fontWeight: weight.medium,
    transitionProperty: "background-color, border-color, color",
  },
  presetOn: { borderColor: color.accent, backgroundColor: surface.selected, color: color.fg },
  presetOff: {
    borderColor: "transparent",
    backgroundColor: { default: null, ":hover": surface.hover },
    color: { default: color.fgMuted, ":hover": color.fg },
  },
});

export function ScheduleForm({ schedule, defaultCwd, onDone, onCancel }: ScheduleFormProps) {
  const t = useT();
  const [draft, setDraft] = useState<ScheduleDraft>(() =>
    initialScheduleDraft(schedule, defaultCwd),
  );
  const { busy, run } = useCommandAction({
    wasRetired: wasGenerationRetired,
    fallback: t("schedules.error.save"),
  });

  const updateDraft = <K extends keyof ScheduleDraft>(key: K, value: ScheduleDraft[K]) => {
    setDraft((current) => ({ ...current, [key]: value }));
  };

  const onSave = () =>
    run(async () => {
      const input = scheduleInputFromDraft(draft);
      if (schedule) {
        await updateSchedule({
          ...input,
          id: schedule.id,
          enabled: schedule.enabled,
          revision: schedule.revision,
        });
      } else {
        await createSchedule(input);
      }
      onDone();
    });

  return (
    <Surface className={stylex.props(ss.stack).className}>
      <TextField
        font="sans"
        value={draft.title}
        onChange={(event) => updateDraft("title", event.target.value)}
        placeholder={t("schedules.form.title")}
        aria-label={t("schedules.form.title")}
      />
      <TextArea
        font="sans"
        size="sm"
        value={draft.instructions}
        onChange={(event) => updateDraft("instructions", event.target.value)}
        rows={4}
        placeholder={t("schedules.form.instructions")}
        aria-label={t("schedules.form.instructions")}
      />
      <div {...stylex.props(ss.lineWrap)}>
        {/* A one-of group, so each option states whether it is the one. The fill was the only
            answer, which a reader that cannot see fill does not get — and the app says this
            with `aria-pressed` in nine other places, including the accent swatches, which are
            the same control with different contents. */}
        {CRON_PRESETS.map((preset) => (
          <Pressable
            key={preset.cron}
            type="button"
            aria-pressed={draft.cron === preset.cron}
            onClick={() => updateDraft("cron", preset.cron)}
            // An edge, because it is the one channel hover cannot forge. The fill alone said
            // this with a 4% black wash against a 3% hover wash, so moving the pointer across
            // the group erased which option it had answered. Accent TEXT would separate them
            // too, and fails: measured on the rendered pixels it is 2.85:1 in dark, where this
            // size needs 4.5. An outlined pill is what the form's own Cancel button already is.
            className={
              stylex.props(
                sf.preset,
                corner.pill,
                draft.cron === preset.cron ? sf.presetOn : sf.presetOff,
                typeStep.uiSm,
              ).className
            }
          >
            {t(preset.key)}
          </Pressable>
        ))}
      </div>
      <TextField
        value={draft.cron}
        onChange={(event) => updateDraft("cron", event.target.value)}
        spellCheck={false}
        placeholder="0 9 * * 1-5"
        aria-label={t("schedules.form.cron")}
      />
      <TextField
        value={draft.cwd}
        onChange={(event) => updateDraft("cwd", event.target.value)}
        spellCheck={false}
        placeholder={t("schedules.form.cwd")}
        aria-label={t("schedules.form.cwd")}
      />
      <div {...stylex.props(vocab.line)}>
        <PillButton
          variant="accent"
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

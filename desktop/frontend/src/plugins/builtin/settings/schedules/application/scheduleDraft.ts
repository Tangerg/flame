import type { ScheduleConfig, ScheduleConfigInput, ScheduleModelSelection } from "./scheduleConfig";

export const CRON_PRESETS: Array<{ key: string; cron: string }> = [
  { key: "schedules.preset.hourly", cron: "0 * * * *" },
  { key: "schedules.preset.daily", cron: "0 9 * * *" },
  { key: "schedules.preset.weekdays", cron: "0 9 * * 1-5" },
  { key: "schedules.preset.weekly", cron: "0 9 * * 1" },
];

export interface ScheduleDraft extends ScheduleConfigInput {
  modelSelection: ScheduleModelSelection | null;
}

export function initialScheduleDraft(
  schedule?: ScheduleConfig,
  defaultCwd?: string,
): ScheduleDraft {
  return {
    title: schedule?.title ?? "",
    instructions: schedule?.instructions ?? "",
    cron: schedule?.cron ?? "0 9 * * 1-5",
    cwd: schedule ? (schedule.cwd ?? "") : (defaultCwd ?? ""),
    modelSelection:
      schedule?.provider && schedule.model
        ? {
            provider: schedule.provider,
            model: schedule.model,
            ...(schedule.reasoningEffort ? { reasoningEffort: schedule.reasoningEffort } : {}),
          }
        : null,
  };
}

export function canSaveScheduleDraft(draft: ScheduleDraft): boolean {
  return draft.instructions.trim() !== "" && draft.cron.trim() !== "";
}

export function scheduleInputFromDraft(
  draft: ScheduleDraft,
  original?: ScheduleDraft,
): ScheduleConfigInput {
  const selection = draft.modelSelection;
  const previous = original?.modelSelection ?? null;
  const selectionChanged =
    selection?.provider !== previous?.provider ||
    selection?.model !== previous?.model ||
    selection?.reasoningEffort !== previous?.reasoningEffort;
  return {
    title: draft.title.trim(),
    instructions: draft.instructions.trim(),
    cwd: draft.cwd.trim(),
    cron: draft.cron.trim(),
    ...(selectionChanged ? { modelSelection: selection } : {}),
  };
}

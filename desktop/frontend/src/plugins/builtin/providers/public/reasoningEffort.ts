import type { Translate } from "@/lib/i18n";

const PUBLISHED_EFFORT_LABEL: Readonly<Record<string, string>> = {
  none: "model.effort.none",
  minimal: "model.effort.minimal",
  low: "model.effort.low",
  medium: "model.effort.medium",
  high: "model.effort.high",
  xhigh: "model.effort.xhigh",
  max: "model.effort.max",
};

// Levels are provider-owned strings; a level outside the common vocabulary is
// shown exactly as the model publishes it rather than guessed at.
export function reasoningEffortLabel(effort: string, t: Translate): string {
  const key = PUBLISHED_EFFORT_LABEL[effort];
  return key === undefined ? effort : t(key);
}

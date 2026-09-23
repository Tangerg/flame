export const UI_FONT_SIZE_DEFAULT_PX = 14;
export const UI_FONT_SIZE_MIN_PX = 11;
export const UI_FONT_SIZE_MAX_PX = 18;

export const UI_TYPE_STEPS = [
  "ui-2xs",
  "ui-xs",
  "ui-sm",
  "ui-md",
  "prose",
  "code",
  "display-sm",
  "display-md",
] as const;

export type UiTypeStep = (typeof UI_TYPE_STEPS)[number];

export function normalizeUiFontSize(value: number | null | undefined): number {
  if (typeof value !== "number" || !Number.isFinite(value)) return UI_FONT_SIZE_DEFAULT_PX;
  return Math.min(UI_FONT_SIZE_MAX_PX, Math.max(UI_FONT_SIZE_MIN_PX, Math.round(value)));
}

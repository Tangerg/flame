// Absolute px, NEVER rem: geometry (header height, row height, gutters) must hold still when
// the type base moves, and a rem ladder drags every padding and width with it.
//
// Only the vocabulary lives here — the ladder that turns a base size into CSS variables is
// the theme context's (`theme/kit/typeLadder`). What keeps these two apart is that `cn()`
// and `iconScale` sit below the plugin layer and cannot import a context.

export const UI_FONT_SIZE_DEFAULT_PX = 14;
export const UI_FONT_SIZE_MIN_PX = 11;
export const UI_FONT_SIZE_MAX_PX = 18;

/** A runtime list, not only a type: `lib/classNames.ts` must name every step for Tailwind
 *  Merge, and a hand-kept copy there silently stops applying when a step is added here. */
export const UI_TYPE_STEPS = ["ui-2xs", "ui-xs", "ui-sm", "ui-md", "prose", "code"] as const;

export type UiTypeStep = (typeof UI_TYPE_STEPS)[number];

export function normalizeUiFontSize(value: number | null | undefined): number {
  if (typeof value !== "number" || !Number.isFinite(value)) return UI_FONT_SIZE_DEFAULT_PX;
  return Math.min(UI_FONT_SIZE_MAX_PX, Math.max(UI_FONT_SIZE_MIN_PX, Math.round(value)));
}

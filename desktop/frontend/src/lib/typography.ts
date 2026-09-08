// Absolute px, NEVER rem: geometry (header height, row height, gutters) must hold still when
// the type base moves, and a rem ladder drags every padding and width with it.
//
// Only the vocabulary lives here — the ladder that turns a base size into CSS variables is
// the theme context's (`theme/kit/typeLadder`). What keeps these two apart is that `cn()`
// and `iconScale` sit below the plugin layer and cannot import a context.

export const UI_FONT_SIZE_DEFAULT_PX = 14;
export const UI_FONT_SIZE_MIN_PX = 11;
export const UI_FONT_SIZE_MAX_PX = 18;

/** A runtime list, not only a type: `theme/kit/typeLadder` walks it to emit one CSS variable
 *  per step, so a step added here reaches the stylesheet without a second list to keep. */
export const UI_TYPE_STEPS = [
  "ui-2xs",
  "ui-xs",
  "ui-sm",
  "ui-md",
  "prose",
  "code",
  // The editorial steps. They had been fixed pixel values on the grounds that a heading is an
  // anchor rather than a scaled thing — which held at the small end and inverted at the large
  // one: at base 18 a `display-sm` heading was 18px above 21px prose, and markdown's own h3 and
  // h5 were smaller than the paragraphs they headed. An anchor that is sometimes below what it
  // anchors is not an anchor; the hierarchy has to be one relationship scaled, not two.
  "markdown-h5",
  "markdown-h3",
  "display-sm",
  "display-md",
  "display-lg",
] as const;

export type UiTypeStep = (typeof UI_TYPE_STEPS)[number];

export function normalizeUiFontSize(value: number | null | undefined): number {
  if (typeof value !== "number" || !Number.isFinite(value)) return UI_FONT_SIZE_DEFAULT_PX;
  return Math.min(UI_FONT_SIZE_MAX_PX, Math.max(UI_FONT_SIZE_MIN_PX, Math.round(value)));
}

// Sizes derive from the type base. Lucide draws on a 24 grid at stroke 2, so a box that grows
// carries a stroke that grows with it — which is right going DOWN (pinning the stroke turns a
// 12px glyph into a blob) and wrong going up: at the 28px step that rule asks for 2.33px, and
// at the largest UI text for 3px, which reads as a heavier icon family sitting beside the
// 12px ones rather than the same family larger. The stroke therefore scales until it reaches
// the weight at which a line stops reading as a hairline, and holds there.

import { normalizeUiFontSize } from "./typography";

export type IconSize = "xs" | "sm" | "md" | "lg" | "xl";

const ICON_SIZES: readonly IconSize[] = ["xs", "sm", "md", "lg", "xl"];

// Offsets from the type base, in px. `xl` is the only multiplier: display glyphs scale with
// the surface rather than tracking a label.
const OFFSETS: Readonly<Record<Exclude<IconSize, "xl">, number>> = {
  xs: -2,
  sm: 0,
  md: 2,
  lg: 6,
};
const XL_RATIO = 2;

/** The rendered weight the stroke stops growing at, in CSS pixels. Above it a line reads as a
 *  filled shape rather than a drawn one, which is the same place Cursor's icon system lands
 *  by a different route: it draws its 24px grid at 1.5 and its 16px grid at 1.25. */
const STROKE_CAP_PX = 1.5;
const LUCIDE_GRID = 24;
const LUCIDE_STROKE = 2;

function iconSizePx(size: IconSize, basePx: number | null | undefined): number {
  const base = normalizeUiFontSize(basePx);
  return size === "xl" ? Math.round(base * XL_RATIO) : base + OFFSETS[size];
}

/**
 * In Lucide's GRID units, not pixels: the attribute is read inside a 24-unit viewBox, so a box
 * of `boxPx` renders `stroke × boxPx / 24`. Capping the rendered pixel therefore means capping
 * the attribute at `CAP × 24 / boxPx` — and taking the smaller of that and Lucide's own 2
 * leaves every step below the cap exactly as it was.
 */
function iconStrokeUnits(boxPx: number): number {
  return Math.min(LUCIDE_STROKE, (STROKE_CAP_PX * LUCIDE_GRID) / boxPx);
}

export function iconScaleCssVariables(
  basePx: number | null | undefined,
): Readonly<Record<string, string>> {
  const variables: Record<string, string> = {};
  for (const size of ICON_SIZES) {
    const box = iconSizePx(size, basePx);
    variables[`--icon-${size}`] = `${box}px`;
    variables[`--icon-stroke-${size}`] = String(Number(iconStrokeUnits(box).toFixed(3)));
  }
  return variables;
}

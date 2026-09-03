// Sizes derive from the type base. Weight does not need a step of its own: Lucide draws on a
// 24 grid at stroke 2, so the stroke scales with the box the same way the glyph does.

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

function iconSizePx(size: IconSize, basePx: number | null | undefined): number {
  const base = normalizeUiFontSize(basePx);
  return size === "xl" ? Math.round(base * XL_RATIO) : base + OFFSETS[size];
}

export function iconScaleCssVariables(
  basePx: number | null | undefined,
): Readonly<Record<string, string>> {
  const variables: Record<string, string> = {};
  for (const size of ICON_SIZES) {
    variables[`--icon-${size}`] = `${iconSizePx(size, basePx)}px`;
  }
  return variables;
}

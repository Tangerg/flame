// Sizes derive from the type base. Weight is NOT: reicon draws filled geometry, not stroked
// paths, so there is no stroke-width to compensate with.

import { normalizeUiFontSize } from "./typography";

export type IconSize = "xs" | "sm" | "md" | "lg" | "xl";

export const ICON_SIZES: readonly IconSize[] = ["xs", "sm", "md", "lg", "xl"];

// Offsets from the type base, in px. `xl` is the only multiplier: display glyphs scale with
// the surface rather than tracking a label.
const OFFSETS: Readonly<Record<Exclude<IconSize, "xl">, number>> = {
  xs: -2,
  sm: 0,
  md: 2,
  lg: 6,
};
const XL_RATIO = 2;

export function iconSizePx(size: IconSize, basePx: number | null | undefined): number {
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

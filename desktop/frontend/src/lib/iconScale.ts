// Sizes derive from the type base, so a glyph beside a label grows when the label does.
//
// Weight is NOT derived here. Reicon draws most of its outlines as filled geometry rather
// than stroked paths, so there is no stroke-width to compensate with — the artwork carries
// its own optical weight and scales with the box.

import { normalizeUiFontSize } from "./typography";

/** `sm` is the default: a glyph beside body text. */
export type IconSize = "xs" | "sm" | "md" | "lg" | "xl";

export const ICON_SIZES: readonly IconSize[] = ["xs", "sm", "md", "lg", "xl"];

// Offsets from the type base, in px. `xl` is the only multiplier: display glyphs
// (empty states, avatars) scale with the whole surface rather than tracking a
// label two steps away.
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

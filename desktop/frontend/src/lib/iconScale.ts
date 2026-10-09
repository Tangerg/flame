import { normalizeUiFontSize } from "./typography";

export type IconSize = "xs" | "sm" | "md" | "lg" | "xl" | "composer";

const ICON_SIZES: readonly IconSize[] = ["xs", "sm", "md", "lg", "xl", "composer"];

const OFFSETS: Readonly<Record<Exclude<IconSize, "xl" | "composer">, number>> = {
  xs: -2,
  sm: 0,
  md: 2,
  lg: 6,
};
const XL_RATIO = 2;

export function iconSizePx(size: IconSize, basePx: number | null | undefined): number {
  if (size === "composer") return 16;
  const base = normalizeUiFontSize(basePx);
  return size === "xl" ? Math.round(base * XL_RATIO) : base + OFFSETS[size];
}

export function iconScaleCssVariables(
  basePx: number | null | undefined,
): Readonly<Record<string, string>> {
  const variables: Record<string, string> = {};
  for (const size of ICON_SIZES) {
    const box = iconSizePx(size, basePx);
    variables[`--icon-${size}`] = `${box}px`;
  }
  return variables;
}

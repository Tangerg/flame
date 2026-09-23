import { normalizeUiFontSize } from "./typography";

export type IconSize = "xs" | "sm" | "md" | "lg" | "xl";

const ICON_SIZES: readonly IconSize[] = ["xs", "sm", "md", "lg", "xl"];

const OFFSETS: Readonly<Record<Exclude<IconSize, "xl">, number>> = {
  xs: -2,
  sm: 0,
  md: 2,
  lg: 6,
};
const XL_RATIO = 2;

const STROKE_CAP_PX = 1.5;
const LUCIDE_GRID = 24;
const LUCIDE_STROKE = 2;

function iconSizePx(size: IconSize, basePx: number | null | undefined): number {
  const base = normalizeUiFontSize(basePx);
  return size === "xl" ? Math.round(base * XL_RATIO) : base + OFFSETS[size];
}

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

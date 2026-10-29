import { blend, rgbColor, wcagContrast, wcagLuminance } from "@/lib/color";
import { type Rgb } from "culori/fn";
import { mixOklab } from "@/features/theme/kit/oklab";
export const WCAG_AA_TEXT = 4.5;

export const WCAG_AA_NON_TEXT = 3;

// Painted channels retain precision until contrast is checked, so rounding cannot create a pass.
export interface RgbColor {
  r: number;
  g: number;
  b: number;
}

export type ContrastColor = string | RgbColor;

function rgb(color: ContrastColor): Rgb {
  return typeof color === "string"
    ? rgbColor(color)!
    : { mode: "rgb", r: color.r / 255, g: color.g / 255, b: color.b / 255 };
}

export function relativeLuminance(color: ContrastColor): number {
  return wcagLuminance(rgb(color));
}

export function contrastRatio(one: ContrastColor, two: ContrastColor): number {
  return wcagContrast(rgb(one), rgb(two));
}

export function compositeColor(ink: string, plane: ContrastColor, opacity: number): RgbColor {
  const color = blend([
    { ...rgb(plane), alpha: 1 },
    { ...rgb(ink), alpha: opacity },
  ]);
  return { r: color.r * 255, g: color.g * 255, b: color.b * 255 };
}

export function inkOnFill(declared: string, fill: string, required: number): string {
  if (contrastRatio(fill, declared) >= required) return declared;
  return contrastRatio(fill, "#ffffff") >= contrastRatio(fill, "#000000") ? "#ffffff" : "#000000";
}

// A control boundary or focus ring sits on whichever plane hosts the control, so it is
// measured against every background the palette emits, not only the canvas.
export function controlEdge(ink: string, canvas: string, planes: readonly ContrastColor[]): string {
  return mixOklab(ink, canvas, legibleMix(ink, canvas, planes, 0, WCAG_AA_NON_TEXT));
}

export function focusRing(accent: string, ink: string, planes: readonly ContrastColor[]): string {
  return mixOklab(ink, accent, legibleMix(ink, accent, planes, 0, WCAG_AA_NON_TEXT));
}

export function legibleMix(
  ink: string,
  base: string,
  against: readonly ContrastColor[],
  floorPct: number,
  required: number,
): number {
  for (let pct = Math.round(floorPct); pct < 100; pct += 1) {
    const mixed = mixOklab(ink, base, pct);
    if (against.every((plane) => contrastRatio(mixed, plane) >= required)) return pct;
  }
  return 100;
}

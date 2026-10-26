import { colord } from "colord";
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

function rgb(color: ContrastColor): RgbColor {
  return typeof color === "string" ? colord(color).toRgb() : color;
}

function relativeLuminance(color: ContrastColor): number {
  const { r, g, b } = rgb(color);
  const channel = (value: number) => {
    const scaled = value / 255;
    return scaled <= 0.04045 ? scaled / 12.92 : ((scaled + 0.055) / 1.055) ** 2.4;
  };
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);
}

export function contrastRatio(one: ContrastColor, two: ContrastColor): number {
  const [lighter, darker] = [relativeLuminance(one), relativeLuminance(two)].sort((a, b) => b - a);
  return (lighter! + 0.05) / (darker! + 0.05);
}

export function compositeColor(ink: string, plane: ContrastColor, opacity: number): RgbColor {
  const foreground = rgb(ink);
  const background = rgb(plane);
  return {
    r: foreground.r * opacity + background.r * (1 - opacity),
    g: foreground.g * opacity + background.g * (1 - opacity),
    b: foreground.b * opacity + background.b * (1 - opacity),
  };
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

import { colord } from "colord";
import { mixOklab } from "./oklab";

export const WCAG_AA_TEXT = 4.5;

export const WCAG_AA_NON_TEXT = 3;

function relativeLuminance(color: string): number {
  const { r, g, b } = colord(color).toRgb();
  const channel = (value: number) => {
    const scaled = value / 255;
    return scaled <= 0.03928 ? scaled / 12.92 : ((scaled + 0.055) / 1.055) ** 2.4;
  };
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);
}

export function contrastRatio(one: string, two: string): number {
  const [lighter, darker] = [relativeLuminance(one), relativeLuminance(two)].sort((a, b) => b - a);
  return (lighter! + 0.05) / (darker! + 0.05);
}

export function inkOnFill(declared: string, fill: string, required: number): string {
  if (contrastRatio(fill, declared) >= required) return declared;
  return contrastRatio(fill, "#ffffff") >= contrastRatio(fill, "#000000") ? "#ffffff" : "#000000";
}

export function edgeOnCanvas(ink: string, canvas: string): string {
  return mixOklab(ink, canvas, legibleMix(ink, canvas, canvas, 0, WCAG_AA_NON_TEXT));
}

export function focusOnCanvas(accent: string, ink: string, canvas: string): string {
  return mixOklab(ink, accent, legibleMix(ink, accent, canvas, 0, WCAG_AA_NON_TEXT));
}

export function legibleMix(
  ink: string,
  base: string,
  against: string,
  floorPct: number,
  required: number,
): number {
  for (let pct = Math.round(floorPct); pct < 100; pct += 1) {
    if (contrastRatio(mixOklab(ink, base, pct), against) >= required) return pct;
  }
  return 100;
}

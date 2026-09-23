import { colord } from "colord";

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

export function focusOnCanvas(accent: string, ink: string, canvas: string): string {
  return mixOklab(ink, accent, legibleMix(ink, accent, canvas, 0, WCAG_AA_NON_TEXT));
}

function toOklab(color: string): [number, number, number] {
  const { r, g, b } = colord(color).toRgb();
  const linear = [r, g, b].map((value) => {
    const scaled = value / 255;
    return scaled <= 0.04045 ? scaled / 12.92 : ((scaled + 0.055) / 1.055) ** 2.4;
  }) as [number, number, number];
  const l = Math.cbrt(
    0.4122214708 * linear[0] + 0.5363325363 * linear[1] + 0.0514459929 * linear[2],
  );
  const m = Math.cbrt(
    0.2119034982 * linear[0] + 0.6806995451 * linear[1] + 0.1073969566 * linear[2],
  );
  const s = Math.cbrt(
    0.0883024619 * linear[0] + 0.2817188376 * linear[1] + 0.6299787005 * linear[2],
  );
  return [
    0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s,
    1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s,
    0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s,
  ];
}

function fromOklab([L, a, b]: [number, number, number]): string {
  const l = (L + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m = (L - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s = (L - 0.0894841775 * a - 1.291485548 * b) ** 3;
  const linear = [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
  ];
  const [r, g, blue] = linear.map((value) => {
    const clamped = Math.min(1, Math.max(0, value));
    const encoded = clamped <= 0.0031308 ? clamped * 12.92 : 1.055 * clamped ** (1 / 2.4) - 0.055;
    return Math.round(encoded * 255);
  }) as [number, number, number];
  return colord({ r, g, b: blue }).toHex();
}

export function mixOklab(from: string, to: string, pct: number): string {
  const [one, two] = [toOklab(from), toOklab(to)];
  const weight = pct / 100;
  return fromOklab([
    one[0] * weight + two[0] * (1 - weight),
    one[1] * weight + two[1] * (1 - weight),
    one[2] * weight + two[2] * (1 - weight),
  ]);
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

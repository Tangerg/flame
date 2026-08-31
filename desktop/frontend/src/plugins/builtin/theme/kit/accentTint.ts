// NOT expressible in CSS: a grey accent has no hue, CSS reads the powerless channel as 0,
// and 0 is RED — so pure black would paint every surface pink. Neutral chroma is
// PROPORTIONAL to the accent's own, so a grey accent yields grey surfaces with no branch.

import { DEFAULT_ACCENT_TINT, type AccentTint } from "./appearance";
import type { NeutralStep } from "@/plugins/sdk";

/** Hue in DEGREES. */
export interface Oklch {
  l: number;
  c: number;
  h: number;
}

const srgbToLinear = (value: number) =>
  value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
const linearToSrgb = (value: number) =>
  value <= 0.0031308 ? 12.92 * value : 1.055 * value ** (1 / 2.4) - 0.055;

export function hexToOklch(hex: string): Oklch {
  const value = Number.parseInt(hex.replace("#", ""), 16);
  const [r, g, b] = [(value >> 16) & 255, (value >> 8) & 255, value & 255].map((channel) =>
    srgbToLinear(channel / 255),
  ) as [number, number, number];
  const l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b);
  const m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b);
  const s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b);
  const lightness = 0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s;
  const a = 1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s;
  const bAxis = 0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s;
  const hue = (Math.atan2(bAxis, a) * 180) / Math.PI;
  return { l: lightness * 100, c: Math.hypot(a, bAxis), h: hue < 0 ? hue + 360 : hue };
}

function toLinearRgb({ l, c, h }: Oklch): [number, number, number] {
  const a = c * Math.cos((h * Math.PI) / 180);
  const b = c * Math.sin((h * Math.PI) / 180);
  const lightness = l / 100;
  const lp = (lightness + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const mp = (lightness - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const sp = (lightness - 0.0894841775 * a - 1.291485548 * b) ** 3;
  return [
    4.0767416621 * lp - 3.3077115913 * mp + 0.2309699292 * sp,
    -1.2684380046 * lp + 2.6097574011 * mp - 0.3413193965 * sp,
    -0.0041960863 * lp - 0.7034186147 * mp + 1.707614701 * sp,
  ];
}

const IN_GAMUT_EPSILON = 1 / 512;

function inSrgbGamut(colour: Oklch): boolean {
  return toLinearRgb(colour).every(
    (channel) => channel >= -IN_GAMUT_EPSILON && channel <= 1 + IN_GAMUT_EPSILON,
  );
}

/** OKLCH → hex, giving up CHROMA rather than hue when a request leaves sRGB. Clamping the
 *  channels lands on whichever saturated first and drags the hue with it. */
export function oklchToHex(colour: Oklch): string {
  let fitted = colour;
  if (!inSrgbGamut(fitted)) {
    let low = 0;
    let high = colour.c;
    for (let step = 0; step < 16; step += 1) {
      const mid = (low + high) / 2;
      if (inSrgbGamut({ ...colour, c: mid })) low = mid;
      else high = mid;
    }
    fitted = { ...colour, c: low };
  }
  return `#${toLinearRgb(fitted)
    .map((channel) => Math.max(0, Math.min(255, Math.round(linearToSrgb(channel) * 255))))
    .map((channel) => channel.toString(16).padStart(2, "0"))
    .join("")}`;
}

const TINT_SCALE: Record<AccentTint, number> = { off: 0, soft: 0.5, standard: 1 };

/** sRGB tops out near C 0.32, 1.6-1.7x a typical reference. */
const MAX_CHROMA_FACTOR = 1.5;

/** The reference is the accent THE THEME DECLARES, not a module constant: that is what makes
 *  an untouched accent reproduce the theme's literals exactly. */
export function neutralChromaFactor(
  accentChroma: number,
  referenceChroma: number,
  tint: AccentTint,
): number {
  if (referenceChroma <= 0) return 0;
  return Math.min(MAX_CHROMA_FACTOR, accentChroma / referenceChroma) * TINT_SCALE[tint];
}

/** Returns HEX so a token map takes it directly and a test reads what will actually paint. */
export function accentTintedNeutral(
  accentHex: string,
  referenceAccentHex: string,
  step: NeutralStep,
  tint: AccentTint = DEFAULT_ACCENT_TINT,
): string {
  const accent = hexToOklch(accentHex);
  const factor = neutralChromaFactor(accent.c, hexToOklch(referenceAccentHex).c, tint);
  // Hue only matters with chroma to carry it; at zero it would read as red on inspection.
  return oklchToHex({ l: step.l, c: step.c * factor, h: factor === 0 ? 0 : accent.h });
}

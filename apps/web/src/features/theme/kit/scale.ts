import { type Scheme } from "@/lib/appearance";
import {
  WCAG_AA_NON_TEXT,
  WCAG_AA_TEXT,
  contrastRatio,
  inkOnFill,
} from "@/features/theme/kit/legibility";
import { fromOklch, mixOklab, toOklch, type Oklch } from "@/features/theme/kit/oklab";
export type Scale = readonly string[];

const WCAG_AAA_TEXT = 7;

const HUE_LIGHTNESS: Record<Scheme, readonly number[]> = {
  light: [0.993, 0.982, 0.957, 0.931, 0.899, 0.859, 0.805, 0.731],
  dark: [0.19, 0.21, 0.266, 0.309, 0.355, 0.403, 0.464, 0.538],
};

const HUE_TEXT_LIGHTNESS: Record<Scheme, readonly [number, number]> = {
  light: [0.552, 0.328],
  dark: [0.774, 0.905],
};

const HUE_CHROMA: Record<Scheme, readonly number[]> = {
  light: [0.02, 0.055, 0.12, 0.21, 0.3, 0.39, 0.5, 0.66, 1, 0.97, 0.88, 0.48],
  dark: [0.1, 0.13, 0.33, 0.47, 0.53, 0.57, 0.62, 0.73, 1, 0.9, 0.7, 0.3],
};

const SOLID_HOVER_SHIFT: Record<Scheme, number> = { light: -0.03, dark: 0.04 };

const NEUTRAL_INK: Record<Scheme, readonly number[]> = {
  light: [0, 2.2, 4.4, 6.6, 9, 12, 17, 26, 46, 51, 74, 100],
  dark: [0, 4, 8.5, 12, 15.5, 20, 27, 39, 46.5, 52.5, 70, 100],
};

export function solidFill(seed: string): string {
  const onWhite = contrastRatio(seed, "#ffffff");
  if (onWhite >= WCAG_AA_TEXT) return seed;
  if (onWhite < WCAG_AA_NON_TEXT && contrastRatio(seed, "#000000") >= WCAG_AA_TEXT) return seed;
  const tone = toOklch(seed);
  let fill = seed;
  for (let l = tone.l; l > 0 && contrastRatio(fill, "#ffffff") < WCAG_AA_TEXT; l -= 0.005)
    fill = fromOklch({ ...tone, l });
  return fill;
}

export const solidInk = (solid: string): string => inkOnFill("#ffffff", solid, WCAG_AA_TEXT);

function solidHover(solid: string, scheme: Scheme, chroma: number): string {
  const tone = toOklch(solid);
  const ink = solidInk(solid);
  const shift = SOLID_HOVER_SHIFT[scheme];
  const preferred = fromOklch({ ...tone, l: tone.l + shift, c: chroma });
  if (contrastRatio(preferred, ink) >= WCAG_AA_TEXT) return preferred;
  const awayFromInk = ink === "#ffffff" ? -1 : 1;
  return fromOklch({ ...tone, l: tone.l + awayFromInk * Math.abs(shift), c: chroma });
}

function legibleTone(start: Oklch, planes: readonly string[], required: number, away: number) {
  let color = fromOklch(start);
  for (
    let l = start.l;
    l > 0 && l < 1 && planes.some((plane) => contrastRatio(color, plane) < required);
    l += away * 0.005
  )
    color = fromOklch({ ...start, l });
  return color;
}

export function hueScale(
  seed: string,
  scheme: Scheme,
  canvas: string,
  textPlanes: readonly string[],
): Scale {
  const solid = solidFill(seed);
  const { c, h } = toOklch(solid);
  const lightness = HUE_LIGHTNESS[scheme];
  const chroma = HUE_CHROMA[scheme];
  const lift = toOklch(canvas).l - lightness[0]!;
  const away = scheme === "light" ? -1 : 1;
  const [lowText, highText] = HUE_TEXT_LIGHTNESS[scheme];
  return [
    ...lightness.map((l, i) => fromOklch({ l: l + lift * (1 - i / 8), c: c * chroma[i]!, h })),
    solid,
    solidHover(solid, scheme, c * chroma[9]!),
    legibleTone({ l: lowText, c: c * chroma[10]!, h }, textPlanes, WCAG_AA_TEXT, away),
    legibleTone({ l: highText, c: c * chroma[11]!, h }, textPlanes, WCAG_AAA_TEXT, away),
  ];
}

export function neutralScale(
  background: string,
  foreground: string,
  scheme: Scheme,
  separation: number,
  textPlaneStep: number,
): Scale {
  const at = (share: number) => mixOklab(foreground, background, Math.min(100, share));
  const steps = NEUTRAL_INK[scheme].map((ink, i) => at(i > 0 && i < 8 ? ink * separation : ink));
  const plane = steps[textPlaneStep - 1]!;
  for (const i of [9, 10]) {
    for (let share = NEUTRAL_INK[scheme][i]!; contrastRatio(steps[i]!, plane) < WCAG_AA_TEXT;) {
      share += 1;
      steps[i] = at(share);
      if (share >= 100) break;
    }
  }
  return steps;
}

export const step = (scale: Scale, n: number): string => scale[n - 1]!;

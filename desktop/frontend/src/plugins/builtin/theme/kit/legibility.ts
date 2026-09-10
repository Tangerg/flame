import { colord } from "colord";

/** The ratio WCAG 2.1 asks of body text. Large text is allowed 3, and a button's label is not. */
export const WCAG_AA_TEXT = 4.5;

/** What 1.4.11 asks of a graphic that carries meaning — a checkmark, a switch's thumb. */
export const WCAG_AA_NON_TEXT = 3;

/**
 * WCAG's relative luminance, which is not `colord`'s `brightness()`.
 *
 * `brightness` is a perceptual weighting meant for "is this light or dark"; the criterion is
 * defined on linearised sRGB with its own coefficients, and the two disagree by enough to move
 * a pair across the threshold. Written out rather than reached for through `colord`'s a11y
 * plugin, whose `extend()` mutates the shared instance for every caller in the app.
 */
function relativeLuminance(color: string): number {
  const { r, g, b } = colord(color).toRgb();
  const channel = (value: number) => {
    const scaled = value / 255;
    return scaled <= 0.03928 ? scaled / 12.92 : ((scaled + 0.055) / 1.055) ** 2.4;
  };
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);
}

/** How far apart two colours read, in the terms the criterion is written in. */
export function contrastRatio(one: string, two: string): number {
  const [lighter, darker] = [relativeLuminance(one), relativeLuminance(two)].sort((a, b) => b - a);
  return (lighter! + 0.05) / (darker! + 0.05);
}

/**
 * The ink that sits ON a fill, when the fill is chosen by someone else.
 *
 * A theme declares one — white, for the blue it ships with — and the accent is a colour the user
 * picks freely, so the two came apart the moment anyone chose a pale one: white on a soft yellow
 * measured 1.39:1. `--color-cta-text` reads the same token, so every primary button went with
 * it, and nothing in the product noticed because the theme's own accent reads fine.
 *
 * The theme's choice is kept whenever it reads, so a palette that has thought about this keeps
 * its answer and the default is untouched. Only a fill that breaks it is overruled, by whichever
 * pole stands furthest from it.
 *
 * `required` is the caller's, because the same ink serves a button's LABEL and a checkbox's
 * MARK, and the criterion asks 4.5 of one and 3 of the other. Passing one threshold for both is
 * how a correct default gets overruled: the accent a mark sits on reads 4.28 against white,
 * which is fine for a graphic and would flip if it were judged as text.
 */
export function inkOnFill(declared: string, fill: string, required: number): string {
  if (contrastRatio(fill, declared) >= required) return declared;
  return contrastRatio(fill, "#ffffff") >= contrastRatio(fill, "#000000") ? "#ffffff" : "#000000";
}

/** sRGB → Oklab, the space every derived ladder in this theme mixes in. */
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

/** What `color-mix(in oklab, from pct%, to)` will resolve to, predicted rather than guessed. */
export function mixOklab(from: string, to: string, pct: number): string {
  const [one, two] = [toOklab(from), toOklab(to)];
  const weight = pct / 100;
  return fromOklab([
    one[0] * weight + two[0] * (1 - weight),
    one[1] * weight + two[1] * (1 - weight),
    one[2] * weight + two[2] * (1 - weight),
  ]);
}

/**
 * The smallest share of `ink` over `fill` that reads, never below what the design already asked
 * for.
 *
 * A ladder written as a fixed percentage guarantees a LOOK, not a ratio: the custom palette's
 * rungs sat at 34% and 53% of the way from the background to the ink, and measured against the
 * background that is 2.4:1 and 4.2:1 — for the product's own dark colours, handed to its own
 * derivation. The hand-written themes clear 5.75 on the same rung, so the bar exists; it was
 * the derivation that could not reach it.
 *
 * Returning the floor unchanged when it already clears is what keeps a palette that reads
 * looking exactly as it did.
 *
 * `base` and `against` are separate because they are: the ladder is MIXED over the page's base
 * colour and READ on the plane a card gives it, which has already stepped toward the ink. A
 * first version searched against the plane and emitted over the base, so it certified a ratio
 * the rendered value did not have — 4.09 where it had promised 4.5.
 */
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

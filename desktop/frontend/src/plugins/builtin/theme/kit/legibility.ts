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

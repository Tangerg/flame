// Reading `globals.css` as text, for the suites that check what it declares against the
// TypeScript that overwrites it at runtime.
//
// Shared because the mirrors have more than one owner: the ladders belong to `lib`, the
// palette and the visual style belong to the theme context, and a context's suite may not
// reach into another's files to borrow a helper.

import { readFileSync } from "node:fs";
import { join } from "node:path";

const CSS = readFileSync(join(process.cwd(), "src/styles/globals.css"), "utf8");

/** First declaration anywhere in the sheet. For a property only `:root` declares. */
export function declaredInCss(property: string): string | undefined {
  return new RegExp(`${property}:\\s*([^;]+);`).exec(CSS)?.[1]?.trim();
}

/**
 * What one selector declares for a property. Required for anything a per-scheme block
 * redeclares.
 *
 * EVERY block with that selector, last one winning, because that is what the cascade does
 * and because the sheet opens `:root` more than once — the palette in one and the motion
 * ladder in another. Reading only the first silently skipped the second, and a comparison
 * that skips is a comparison that passes.
 */
export function declaredInBlock(selector: string, property: string): string | undefined {
  const pattern = new RegExp(`^\\s*${property}:\\s*([^;]+);`, "m");
  let declared: string | undefined;
  for (
    let at = CSS.indexOf(`${selector} {`);
    at !== -1;
    at = CSS.indexOf(`${selector} {`, at + 1)
  ) {
    const found = pattern.exec(CSS.slice(at, CSS.indexOf("\n}", at)))?.[1];
    if (found !== undefined) declared = found.trim();
  }
  return declared;
}

/** Compared as CSS, not as text: the formatter wraps a long `color-mix()` across lines, and
 *  where it broke is not a difference in the value. */
export function collapse(value: string): string {
  return value.replaceAll(/\s+/g, " ").replaceAll("( ", "(").replaceAll(" )", ")").trim();
}

/**
 * Which of `contributed` the block declares differently.
 *
 * Only what the block DECLARES is compared: a token the dark block omits inherits the light
 * one on purpose, and demanding it here would be asking the stylesheet to repeat itself.
 * `compared` is returned so a caller can refuse a run where the lookup matched nothing —
 * a renamed selector would otherwise make every comparison silently vacuous.
 */
export function driftAgainstBlock(
  selector: string,
  contributed: Readonly<Record<string, string>>,
): { compared: number; disagreed: string[] } {
  const disagreed: string[] = [];
  let compared = 0;
  for (const [name, value] of Object.entries(contributed)) {
    const declared = declaredInBlock(selector, `--${name}`);
    if (declared === undefined) continue;
    compared++;
    if (collapse(declared) !== collapse(value)) {
      disagreed.push(`--${name}: css=${collapse(declared)} spec=${collapse(value)}`);
    }
  }
  return { compared, disagreed };
}

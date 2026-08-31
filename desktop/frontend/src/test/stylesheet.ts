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

/** The declaration inside one block. Required for anything a per-scheme block redeclares. */
export function declaredInBlock(selector: string, property: string): string | undefined {
  const start = CSS.indexOf(`${selector} {`);
  if (start === -1) return undefined;
  const block = CSS.slice(start, CSS.indexOf("\n}", start));
  return new RegExp(`^\\s*${property}:\\s*([^;]+);`, "m").exec(block)?.[1]?.trim();
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

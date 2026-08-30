// Normalization applied BEFORE remark-math parses: models emit math in delimiters it does
// not recognize, and write currency (`$5`) that single-`$` math would otherwise eat.
//
// STREAMING-SAFE: each transform runs on the full accumulated text and rewrites only
// COMPLETE delimiter pairs, so a half-arrived `\(` waits for its closer.

// One OR two leading backslashes: models emit both depending on how the JSON transport
// escaped the response. No newline inside inline math — that is a paragraph break.
const LATEX_INLINE_DELIMITER = /\\{1,2}\(([^\n]+?)\\{1,2}\)/g;
const LATEX_DISPLAY_DELIMITER = /\\{1,2}\[([\s\S]+?)\\{1,2}\]/g;

/** remark-math recognizes only the dollar form, so without this rewrite bracket math
 *  renders as literal text. A single or double leading backslash is accepted. */
export function rewriteLatexBracketDelimiters(text: string): string {
  return text
    .replace(LATEX_INLINE_DELIMITER, (_, body: string) => `$${body.trim()}$`)
    .replace(LATEX_DISPLAY_DELIMITER, (_, body: string) => `$$${body.trim()}$$`);
}

const MATH_TAG = /\[\/math\]([\s\S]*?)\[\/math\]/g;
const INLINE_TAG = /\[\/inline\]([\s\S]*?)\[\/inline\]/g;

export function rewriteCustomMathTags(text: string): string {
  return text
    .replace(MATH_TAG, (_, body: string) => `$$${body.trim()}$$`)
    .replace(INLINE_TAG, (_, body: string) => `$${body.trim()}$`);
}

// Group 1 anchors on start-of-string or a char that is neither backslash nor `$`, leaving
// `$$` display math intact; group 2 captures an EVEN-length backslash run so an
// already-escaped `\$` (odd run) is not double-escaped. Math almost always opens with a
// letter or `\command`, so a digit after `$` is treated as currency.
const CURRENCY_DOLLAR = /(^|[^\\$])((?:\\\\)*)\$(?=\d)/g;

/** Known trade-off: an inline expression genuinely opening with a digit (`$5x = 10$`) has
 *  its leading `$` escaped too. Rare, and the bracket rewrites are unaffected because they
 *  run after escaping. */
export function escapeCurrencyDollars(text: string): string {
  return text.replace(CURRENCY_DOLLAR, "$1$2\\$");
}

export function normalizeMathDelimiters(text: string): string {
  return rewriteLatexBracketDelimiters(rewriteCustomMathTags(text));
}

/** Currency escaping runs BEFORE the bracket rewrites so `\(5\)` survives: at that point
 *  there is no `$` for the currency rule to see, and the rewrite then yields `$5$`. */
export function normalizeMarkdownMath(text: string): string {
  return normalizeMathDelimiters(escapeCurrencyDollars(text));
}

// Either bound trips it: a TALL paste that would scroll the capped textarea, or a WIDE
// single-line blob. Deliberately conservative — a miss leaves the text inline and blocks
// nothing.

export const LARGE_PASTE_LINES = 12;
export const LARGE_PASTE_CHARS = 1600;

export function countLines(text: string): number {
  return text.split("\n").length;
}

export function isLargePaste(text: string): boolean {
  return text.length >= LARGE_PASTE_CHARS || countLines(text) >= LARGE_PASTE_LINES;
}

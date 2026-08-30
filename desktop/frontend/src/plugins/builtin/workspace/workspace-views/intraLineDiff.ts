// Common-prefix + common-suffix trim, not a word-LCS: two scans, allocation-free. It can
// OVER-mark — two disjoint edits collapse into one span — but never under-mark, which is
// what makes the cheap version safe here.

/** Changed sub-ranges `[start, end)` on each side of a replaced line, or null
 *  when that side has no marked region (a pure insertion / deletion, or the
 *  whole line changed — the row tint already conveys a wholesale change). */
export interface IntraLineDiff {
  del: [number, number] | null;
  add: [number, number] | null;
}

const isHighSurrogate = (unit: number) => unit >= 0xd800 && unit <= 0xdbff;
const isLowSurrogate = (unit: number) => unit >= 0xdc00 && unit <= 0xdfff;

/** Whether the boundary at `index` falls between the two halves of one character. */
function splitsPair(text: string, index: number): boolean {
  return isHighSurrogate(text.charCodeAt(index - 1)) && isLowSurrogate(text.charCodeAt(index));
}

export function intraLineDiff(a: string, b: string): IntraLineDiff {
  if (a === b) return { del: null, add: null };
  const max = Math.min(a.length, b.length);
  let p = 0;
  while (p < max && a[p] === b[p]) p++;
  let s = 0;
  while (s < max - p && a[a.length - 1 - s] === b[b.length - 1 - s]) s++;

  // The scans compare UTF-16 code units, so two characters sharing a leading
  // surrogate — every emoji on the same plane does — leave a boundary inside one
  // character. The marked range becomes a Shiki decoration offset, and half a
  // character is not a position it can decorate. Both corrections widen the range,
  // which is the direction this function is already allowed to be wrong in.
  if (p > 0 && (splitsPair(a, p) || splitsPair(b, p))) p--;
  while (s > 0 && (splitsPair(a, a.length - s) || splitsPair(b, b.length - s))) s--;

  // No shared prefix OR suffix → the lines are wholesale different; the row
  // tint says that already, so adding a word mark over the entire line is noise.
  if (p === 0 && s === 0) return { del: null, add: null };
  const delEnd = a.length - s;
  const addEnd = b.length - s;
  return {
    del: delEnd > p ? [p, delEnd] : null,
    add: addEnd > p ? [p, addEnd] : null,
  };
}

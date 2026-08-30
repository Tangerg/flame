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

export function intraLineDiff(a: string, b: string): IntraLineDiff {
  if (a === b) return { del: null, add: null };
  const max = Math.min(a.length, b.length);
  let p = 0;
  while (p < max && a[p] === b[p]) p++;
  let s = 0;
  while (s < max - p && a[a.length - 1 - s] === b[b.length - 1 - s]) s++;
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

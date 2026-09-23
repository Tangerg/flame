export interface IntraLineDiff {
  del: [number, number] | null;
  add: [number, number] | null;
}

const isHighSurrogate = (unit: number) => unit >= 0xd800 && unit <= 0xdbff;
const isLowSurrogate = (unit: number) => unit >= 0xdc00 && unit <= 0xdfff;

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

  if (p > 0 && (splitsPair(a, p) || splitsPair(b, p))) p--;
  while (s > 0 && (splitsPair(a, a.length - s) || splitsPair(b, b.length - s))) s--;

  if (p === 0 && s === 0) return { del: null, add: null };
  const delEnd = a.length - s;
  const addEnd = b.length - s;
  return {
    del: delEnd > p ? [p, delEnd] : null,
    add: addEnd > p ? [p, addEnd] : null,
  };
}

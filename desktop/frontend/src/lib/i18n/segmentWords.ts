// Latin runs as words, CJK as individual codepoints, trailing punctuation glued to the
// preceding token. Backed by Intl.Segmenter, always available in the WebView Wails ships.

const segmenter = new Intl.Segmenter(undefined, { granularity: "word" });

const TRAILING_PUNCT_RE = /^[，。！？,!?]/;

export function segmentWords(text: string): string[] {
  const out: string[] = [];
  for (const { segment } of segmenter.segment(text)) {
    if (TRAILING_PUNCT_RE.test(segment) && out.length > 0) {
      out[out.length - 1] += segment;
    } else if (segment.length > 0) {
      out.push(segment);
    }
  }
  return out;
}

import { segmentWords } from "@/lib/i18n/segmentWords";

export function createStreamWords() {
  let source = "";
  let words: string[] = [];
  let stableCount = 0;
  let stableOffset = 0;
  return {
    get words(): readonly string[] {
      return words;
    },
    update(next: string): { changedFrom: number; changedAt: number } {
      if (!next.startsWith(source)) {
        stableCount = 0;
        stableOffset = 0;
      }
      source = next;
      const changedFrom = stableCount;
      const changedAt = stableOffset;
      const tail = segmentWords(next.slice(stableOffset));
      words = words.slice(0, stableCount).concat(tail);
      let offset = changedAt;
      for (const [index, word] of tail.entries()) {
        // ICU has no incremental state API. A hard line break closes preceding context;
        // retain the break itself because segmentWords may attach following punctuation.
        if (word.includes("\n")) {
          stableCount = changedFrom + index;
          stableOffset = offset;
        }
        offset += word.length;
      }
      return { changedFrom, changedAt };
    },
  };
}

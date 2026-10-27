import type { ComparisonRow } from "./diff-table";

type SourceLine = Exclude<ComparisonRow, { type: "hunk" }>;
interface WordPart {
  text: string;
  changed: boolean;
}
export interface PresentedLine {
  source: SourceLine;
  sourceIndex: number;
  parts?: WordPart[];
}
type PresentedRow =
  | { type: "hunk"; text: string; key: number }
  | {
      type: "code";
      before?: PresentedLine;
      after?: PresentedLine;
      key: number;
    };

const segmenter = new Intl.Segmenter("und", { granularity: "word" });
// Bound advisory emphasis, not the source, for long or token-dense lines.
const MAX_TOKENS = 128;

function words(text: string): string[] | undefined {
  const tokens: string[] = [];
  for (const { segment } of segmenter.segment(text)) {
    if (tokens.length === MAX_TOKENS) return;
    tokens.push(segment);
  }
  return tokens;
}

function parts(tokens: string[], unchanged: boolean[]): WordPart[] {
  const result: WordPart[] = [];
  for (const [index, text] of tokens.entries()) {
    const changed = !unchanged[index];
    const previous = result.at(-1);
    if (previous?.changed === changed) previous.text += text;
    else result.push({ text, changed });
  }
  return result;
}

function emphasize(before: PresentedLine, after: PresentedLine): void {
  if (before.source.code === after.source.code) return;
  const oldWords = words(before.source.code);
  const newWords = words(after.source.code);
  if (!oldWords || !newWords) {
    before.parts = [{ text: before.source.code, changed: true }];
    after.parts = [{ text: after.source.code, changed: true }];
    return;
  }
  const columns = newWords.length + 1;
  const common = new Uint16Array((oldWords.length + 1) * columns);
  for (let left = oldWords.length - 1; left >= 0; left--) {
    for (let right = newWords.length - 1; right >= 0; right--) {
      common[left * columns + right] =
        oldWords[left] === newWords[right]
          ? 1 + common[(left + 1) * columns + right + 1]!
          : Math.max(
              common[(left + 1) * columns + right]!,
              common[left * columns + right + 1]!,
            );
    }
  }
  const oldUnchanged: boolean[] = [];
  const newUnchanged: boolean[] = [];
  let left = 0;
  let right = 0;
  while (left < oldWords.length && right < newWords.length) {
    if (oldWords[left] === newWords[right]) {
      oldUnchanged[left++] = true;
      newUnchanged[right++] = true;
    } else if (
      common[(left + 1) * columns + right]! >=
      common[left * columns + right + 1]!
    ) {
      left++;
    } else {
      right++;
    }
  }
  before.parts = parts(oldWords, oldUnchanged);
  after.parts = parts(newWords, newUnchanged);
}

export function presentDiffRows(
  rows: readonly ComparisonRow[],
  split: boolean,
): PresentedRow[] {
  const result: PresentedRow[] = [];
  let index = 0;
  while (index < rows.length) {
    const row = rows[index]!;
    if (row.type === "hunk") {
      result.push({ ...row, key: index });
      index++;
      continue;
    }
    if (row.type === "context") {
      const line = { source: row, sourceIndex: index };
      result.push({ type: "code", before: line, after: line, key: index });
      index++;
      continue;
    }
    const before: PresentedLine[] = [];
    const after: PresentedLine[] = [];
    while (rows[index]?.type === "deleted") {
      before.push({ source: rows[index] as SourceLine, sourceIndex: index++ });
    }
    while (rows[index]?.type === "added") {
      after.push({ source: rows[index] as SourceLine, sourceIndex: index++ });
    }
    for (let pair = 0; pair < Math.min(before.length, after.length); pair++) {
      emphasize(before[pair]!, after[pair]!);
    }
    if (split) {
      for (let pair = 0; pair < Math.max(before.length, after.length); pair++) {
        result.push({
          type: "code",
          before: before[pair],
          after: after[pair],
          key: (before[pair] ?? after[pair])!.sourceIndex,
        });
      }
    } else {
      for (const line of before)
        result.push({ type: "code", before: line, key: line.sourceIndex });
      for (const line of after)
        result.push({ type: "code", after: line, key: line.sourceIndex });
    }
  }
  return result;
}

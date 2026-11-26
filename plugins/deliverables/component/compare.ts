import { diffArrays, diffWords } from "diff";
import type {
  BlockView,
  DocumentLayout,
  ElementView,
  SheetCellView,
  SheetView,
  SlideLayout,
  SlideView,
  WorkbookLayout,
} from "../contracts/layouts";
import { same } from "./live-draft";
import { blockText } from "./docs-draft";
import { pointOf } from "./sheets-grid";

// Comparisons are derived from two revisions' projections. Identities follow
// each projection's own rules: slide, element, run, sheet and cell identities
// are stable, while document blocks are positions and are aligned by content.

export type Segment = { text: string; change?: "added" | "removed" };

// Comparison runs on the interface thread; a span that cannot be aligned within
// this budget shows as replaced instead of blocking the view.
const BUDGET_MS = 100;
const WORDS = new Intl.Segmenter(undefined, { granularity: "word" });

function replaced(before: string, after: string): Segment[] {
  return [
    ...(before ? [{ text: before, change: "removed" as const }] : []),
    ...(after ? [{ text: after, change: "added" as const }] : []),
  ];
}

// Word segmentation keeps Chinese and Japanese changes to whole words.
export function textChanges(before: string, after: string): Segment[] {
  const changes = diffWords(before, after, {
    intlSegmenter: WORDS,
    timeout: BUDGET_MS,
  });
  if (!changes) return replaced(before, after);
  return changes.map((change): Segment =>
    change.added
      ? { text: change.value, change: "added" }
      : change.removed
        ? { text: change.value, change: "removed" }
        : { text: change.value },
  );
}

type Pair<T> =
  | { before: T; after: T }
  | { before: T; after?: undefined }
  | { before?: undefined; after: T };

function pairs<T>(
  before: T[],
  after: T[],
  equal: (left: T, right: T) => boolean,
): Pair<T>[] {
  const changes = diffArrays(before, after, {
    comparator: equal,
    timeout: BUDGET_MS,
  });
  if (!changes)
    return [
      ...before.map((item) => ({ before: item })),
      ...after.map((item) => ({ after: item })),
    ];
  const result: Pair<T>[] = [];
  let old = 0;
  let current = 0;
  for (const change of changes)
    for (let index = 0; index < change.count; index++) {
      if (change.removed) result.push({ before: before[old++]! });
      else if (change.added) result.push({ after: after[current++]! });
      else result.push({ before: before[old++]!, after: after[current++]! });
    }
  return result;
}

// Identical items anchor the alignment; inside each changed span, similar
// items pair as edits and the rest are removed or added.
function align<T>(
  before: T[],
  after: T[],
  identical: (left: T, right: T) => boolean,
  similar: (left: T, right: T) => boolean,
): Pair<T>[] {
  const result: Pair<T>[] = [];
  let removed: T[] = [];
  let added: T[] = [];
  const flush = () => {
    result.push(...pairs(removed, added, similar));
    removed = [];
    added = [];
  };
  for (const pair of pairs(before, after, identical)) {
    if (pair.before !== undefined && pair.after !== undefined) {
      flush();
      result.push(pair);
    } else if (pair.before !== undefined) removed.push(pair.before);
    else added.push(pair.after!);
  }
  flush();
  return result;
}

// Dice similarity of character bigrams: language-neutral and tolerant of
// several separate edits in one paragraph.
function similarity(left: string, right: string): number {
  if (left === right) return 1;
  if (left.length < 2 || right.length < 2) return 0;
  const counts = new Map<string, number>();
  for (let index = 0; index < left.length - 1; index++) {
    const pair = left.slice(index, index + 2);
    counts.set(pair, (counts.get(pair) ?? 0) + 1);
  }
  let shared = 0;
  for (let index = 0; index < right.length - 1; index++) {
    const pair = right.slice(index, index + 2);
    const count = counts.get(pair);
    if (!count) continue;
    shared++;
    counts.set(pair, count - 1);
  }
  return (2 * shared) / (left.length + right.length - 2);
}

export type TextChange = { id: string; before: string; after: string };

function runChanges(
  before: { id: string; text: string }[],
  after: { id: string; text: string }[],
): TextChange[] {
  const old = new Map(before.map((run) => [run.id, run.text]));
  const current = new Set(after.map((run) => run.id));
  return [
    ...after.map((run) => ({
      id: run.id,
      before: old.get(run.id) ?? "",
      after: run.text,
    })),
    ...before
      .filter((run) => !current.has(run.id))
      .map((run) => ({ id: run.id, before: run.text, after: "" })),
  ].filter((change) => change.before !== change.after);
}

export type ElementChange =
  | { type: "added" | "removed"; element: ElementView }
  | {
      type: "changed";
      element: ElementView;
      before: ElementView;
      text: TextChange[];
      frame: boolean;
      chart: boolean;
      legend: boolean;
    };

export type SlideChange = {
  slide: SlideView;
  status:
    { type: "added" | "removed" | "kept" } | { type: "moved"; from: number };
  notes: TextChange[];
  elements: ElementChange[];
};

function elementChanges(before: SlideView, after: SlideView): ElementChange[] {
  const old = new Map(before.elements.map((element) => [element.id, element]));
  const current = new Set(after.elements.map((element) => element.id));
  return [
    ...after.elements.flatMap((element): ElementChange[] => {
      const previous = old.get(element.id);
      if (!previous) return [{ type: "added", element }];
      const text = runChanges(previous.text, element.text);
      const frame = !same(previous.frame ?? null, element.frame ?? null);
      const chart = !same(
        previous.chart?.data ?? null,
        element.chart?.data ?? null,
      );
      const legend =
        previous.chart?.legendPosition !== element.chart?.legendPosition;
      return text.length || frame || chart || legend
        ? [
            {
              type: "changed",
              element,
              before: previous,
              text,
              frame,
              chart,
              legend,
            },
          ]
        : [];
    }),
    ...before.elements
      .filter((element) => !current.has(element.id))
      .map((element): ElementChange => ({ type: "removed", element })),
  ];
}

// A slide moved when its order among the slides both revisions keep changed.
export function compareSlides(
  before: SlideLayout,
  after: SlideLayout,
): SlideChange[] {
  const old = new Map(before.slides.map((slide) => [slide.id, slide]));
  const current = new Set(after.slides.map((slide) => slide.id));
  const kept = (slides: SlideView[]) =>
    slides
      .filter((slide) => old.has(slide.id) && current.has(slide.id))
      .map((slide) => slide.id);
  const moved = new Set(
    pairs(
      kept(before.slides),
      kept(after.slides),
      (left, right) => left === right,
    ).flatMap((pair) => (pair.before === undefined ? [pair.after] : [])),
  );
  return [
    ...after.slides.flatMap((slide): SlideChange[] => {
      const previous = old.get(slide.id);
      if (!previous)
        return [{ slide, status: { type: "added" }, notes: [], elements: [] }];
      const change: SlideChange = {
        slide,
        status: moved.has(slide.id)
          ? { type: "moved", from: previous.number }
          : { type: "kept" },
        notes: runChanges(previous.notes, slide.notes),
        elements: elementChanges(previous, slide),
      };
      return change.status.type === "moved" ||
        change.notes.length ||
        change.elements.length
        ? [change]
        : [];
    }),
    ...before.slides
      .filter((slide) => !current.has(slide.id))
      .map((slide): SlideChange => ({
        slide,
        status: { type: "removed" },
        notes: [],
        elements: [],
      })),
  ];
}

export type CellChange = {
  ref: string;
  before?: SheetCellView;
  after?: SheetCellView;
};
export type SheetChange = { sheet: SheetView; cells: CellChange[] };

const cellValue = (cell: SheetCellView | undefined) => ({
  input: cell?.input ?? null,
  shown: cell?.shown ?? { type: "blank" },
});

// A sheet only one revision has compares against an empty sheet.
export function compareSheets(
  before: WorkbookLayout,
  after: WorkbookLayout,
): SheetChange[] {
  const current = new Set(after.sheets.map((sheet) => sheet.id));
  const sheets = [
    ...after.sheets,
    ...before.sheets.filter((sheet) => !current.has(sheet.id)),
  ];
  return sheets.flatMap((sheet): SheetChange[] => {
    const cellsOf = (layout: WorkbookLayout) =>
      new Map(
        layout.sheets
          .find((entry) => entry.id === sheet.id)
          ?.cells.map((cell) => [cell.ref, cell]),
      );
    const old = cellsOf(before);
    const now = cellsOf(after);
    const cells = [...new Set([...now.keys(), ...old.keys()])]
      .map((ref) => ({ ref, before: old.get(ref), after: now.get(ref) }))
      .filter(
        (change) => !same(cellValue(change.before), cellValue(change.after)),
      )
      .sort((left, right) => {
        const a = pointOf(left.ref);
        const b = pointOf(right.ref);
        return a.row - b.row || a.column - b.column;
      });
    return cells.length ? [{ sheet, cells }] : [];
  });
}

export type BlockChange =
  | { type: "added" | "removed"; block: BlockView }
  | { type: "changed"; block: BlockView; before: BlockView };

const SIMILAR = 0.5;

// Block identities are positions, so blocks are aligned by their exact XML and
// then paired by text within each changed span, in document order.
export function compareBlocks(
  before: DocumentLayout,
  after: DocumentLayout,
): BlockChange[] {
  return align(
    before.blocks,
    after.blocks,
    (left, right) => left.expect === right.expect,
    (left, right) =>
      left.kind === right.kind &&
      similarity(blockText(left), blockText(right)) >= SIMILAR,
  ).flatMap((pair): BlockChange[] => {
    if (pair.before === undefined)
      return [{ type: "added", block: pair.after }];
    if (pair.after === undefined)
      return [{ type: "removed", block: pair.before }];
    return pair.before.expect === pair.after.expect
      ? []
      : [{ type: "changed", block: pair.after, before: pair.before }];
  });
}

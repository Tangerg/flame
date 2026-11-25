import type { DocumentOperation } from "../contracts/content";
import type {
  BlockView,
  DocumentLayout,
  ParagraphView,
} from "../contracts/layouts";
import { edited, same, type Draft, type Reconciliation } from "./live-draft";

export type DocOperation = Exclude<
  DocumentOperation,
  { type: "appendParagraph" }
>;
type Of<T extends DocOperation["type"]> = Extract<DocOperation, { type: T }>;
export type InsertedParagraph = Of<"insertParagraph">;
export type ParagraphStyle = Of<"setParagraphStyle">["style"];
export const PARAGRAPH_STYLES: ParagraphStyle[] = [
  "Normal",
  "Heading1",
  "Heading2",
  "Heading3",
];

// A paragraph that names no style renders as Normal.
export const styleOf = (style: string) => style || "Normal";

export const textOf = (runs: { text: string }[]) =>
  runs.map((run) => run.text).join("");

const isHigh = (code: number) => code >= 0xd800 && code <= 0xdbff;
const isLow = (code: number) => code >= 0xdc00 && code <= 0xdfff;

// The changed span between two texts, never splitting a surrogate pair.
function changedSpan(current: string, next: string) {
  const limit = Math.min(current.length, next.length);
  let prefix = 0;
  while (prefix < limit && current[prefix] === next[prefix]) prefix++;
  if (prefix > 0 && isHigh(current.charCodeAt(prefix - 1))) prefix--;
  let suffix = 0;
  while (
    suffix < limit - prefix &&
    current[current.length - 1 - suffix] === next[next.length - 1 - suffix]
  )
    suffix++;
  if (suffix > 0 && isLow(current.charCodeAt(current.length - suffix)))
    suffix--;
  return {
    start: prefix,
    end: current.length - suffix,
    text: next.slice(prefix, next.length - suffix),
  };
}

// The first touched run takes the new text and later touched runs empty, so
// untouched runs keep their formatting. Typing at a run boundary continues the
// run before it.
export function runTexts(runs: string[], next: string): string[] {
  const span = changedSpan(runs.join(""), next);
  const stops = runs.map((_, index) =>
    runs.slice(0, index + 1).reduce((total, text) => total + text.length, 0),
  );
  const first =
    span.start === span.end
      ? stops.findIndex((stop) => stop >= span.start)
      : stops.findIndex((stop) => stop > span.start);
  const last =
    span.start === span.end
      ? first
      : stops.findIndex((stop) => stop >= span.end);
  return runs.map((text, index) => {
    const start = stops[index]! - text.length;
    if (index < first || index > last) return text;
    const head = index === first ? text.slice(0, span.start - start) : "";
    const tail = index === last ? text.slice(span.end - start) : "";
    return index === first ? head + span.text + tail : tail;
  });
}

function commit(draft: Draft<DocOperation>, operations: DocOperation[]) {
  return same(operations, draft.operations) ? draft : edited(draft, operations);
}

const targets = (operation: DocOperation, id: string) =>
  operation.type !== "insertParagraph" && operation.id === id;

export function editText(
  draft: Draft<DocOperation>,
  paragraph: ParagraphView,
  text: string,
): Draft<DocOperation> {
  const texts = runTexts(
    paragraph.runs.map((run) => run.text),
    text,
  );
  return commit(draft, [
    ...draft.operations.filter(
      (operation) => !paragraph.runs.some((run) => targets(operation, run.id)),
    ),
    ...paragraph.runs.flatMap((run, index): DocOperation[] =>
      texts[index] === run.text
        ? []
        : [
            {
              type: "replaceText",
              id: run.id,
              expect: run.text,
              text: texts[index]!,
            },
          ],
    ),
  ]);
}

export function setStyle(
  draft: Draft<DocOperation>,
  paragraph: ParagraphView,
  style: ParagraphStyle,
): Draft<DocOperation> {
  const others = draft.operations.filter(
    (operation) =>
      !(
        operation.type === "setParagraphStyle" && operation.id === paragraph.id
      ),
  );
  return commit(
    draft,
    styleOf(paragraph.style) === style
      ? others
      : [
          ...others,
          {
            type: "setParagraphStyle",
            id: paragraph.id,
            expect: paragraph.expect,
            style,
          },
        ],
  );
}

// Deleting a block retires edits inside it, which would overlap the deletion.
export function remove(
  draft: Draft<DocOperation>,
  block: BlockView,
): Draft<DocOperation> {
  const inside = (operation: DocOperation) =>
    targets(operation, block.id) ||
    (block.kind === "paragraph" &&
      block.runs.some((run) => targets(operation, run.id)));
  return commit(draft, [
    ...draft.operations.filter((operation) => !inside(operation)),
    { type: "deleteBlock", id: block.id, expect: block.expect },
  ]);
}

export function restore(
  draft: Draft<DocOperation>,
  block: BlockView,
): Draft<DocOperation> {
  return commit(
    draft,
    draft.operations.filter(
      (operation) =>
        !(operation.type === "deleteBlock" && operation.id === block.id),
    ),
  );
}

export function insert(
  draft: Draft<DocOperation>,
  after: BlockView | undefined,
): Draft<DocOperation> {
  return commit(draft, [
    ...draft.operations,
    after
      ? {
          type: "insertParagraph",
          after: after.id,
          expect: after.expect,
          text: "",
        }
      : { type: "insertParagraph", text: "" },
  ]);
}

export function changeInserted(
  draft: Draft<DocOperation>,
  index: number,
  change: Partial<Pick<InsertedParagraph, "text" | "style">> | undefined,
): Draft<DocOperation> {
  return commit(
    draft,
    draft.operations.flatMap((operation, position) =>
      position !== index || operation.type !== "insertParagraph"
        ? [operation]
        : change
          ? [{ ...operation, ...change }]
          : [],
    ),
  );
}

// A saved item shows its draft state; operations are always recorded against
// the saved block, whose values are the preconditions.
export type DocItem =
  | {
      source: "saved";
      block: BlockView;
      saved: BlockView;
      deleted: boolean;
      changed: boolean;
    }
  | { source: "inserted"; index: number; paragraph: InsertedParagraph };

function applied(block: BlockView, operations: DocOperation[]): BlockView {
  if (block.kind !== "paragraph") return block;
  const text = new Map(
    operations.flatMap((operation) =>
      operation.type === "replaceText" ? [[operation.id, operation.text]] : [],
    ),
  );
  const style = operations.find(
    (operation) =>
      operation.type === "setParagraphStyle" && operation.id === block.id,
  ) as Of<"setParagraphStyle"> | undefined;
  return {
    ...block,
    style: style?.style ?? block.style,
    runs: block.runs.map((run) => ({
      ...run,
      text: text.get(run.id) ?? run.text,
    })),
  };
}

// Inserted paragraphs follow the block they name, in draft order.
export function preview(
  layout: DocumentLayout,
  operations: DocOperation[],
): DocItem[] {
  const inserted = (after: string | undefined): DocItem[] =>
    operations.flatMap((operation, index) =>
      operation.type === "insertParagraph" && operation.after === after
        ? [{ source: "inserted", index, paragraph: operation }]
        : [],
    );
  return [
    ...inserted(undefined),
    ...layout.blocks.flatMap((block): DocItem[] => {
      const shown = applied(block, operations);
      return [
        {
          source: "saved",
          block: shown,
          saved: block,
          deleted: operations.some(
            (operation) =>
              operation.type === "deleteBlock" && operation.id === block.id,
          ),
          changed: !same(shown, block),
        },
        ...inserted(block.id),
      ];
    }),
  ];
}

export function blockText(block: BlockView): string {
  switch (block.kind) {
    case "paragraph":
      return textOf(block.runs);
    case "table":
      return block.rows.map((row) => row.join(" · ")).join(" / ");
    case "other":
      return block.text;
  }
}

function reconcileText(
  head: DocumentLayout,
  operation: Of<"replaceText">,
): Reconciliation<DocOperation> {
  const run = head.blocks
    .flatMap((block) => (block.kind === "paragraph" ? block.runs : []))
    .find((entry) => entry.id === operation.id);
  if (!run) return { operation, status: "missing" };
  if (run.text === operation.text)
    return { operation, status: "applied", rebased: operation };
  if (run.text === operation.expect)
    return { operation, status: "clean", rebased: operation };
  return {
    operation,
    status: "conflict",
    rebased: operation,
    current: run.text,
  };
}

function reconcileBlock(
  head: DocumentLayout,
  operation: Exclude<DocOperation, Of<"replaceText">>,
  id: string,
  expect: string,
): Reconciliation<DocOperation> {
  const block = head.blocks.find((entry) => entry.id === id);
  if (!block) return { operation, status: "missing" };
  if (
    operation.type === "setParagraphStyle" &&
    block.kind === "paragraph" &&
    styleOf(block.style) === operation.style
  )
    return { operation, status: "applied", rebased: operation };
  if (block.expect === expect)
    return { operation, status: "clean", rebased: operation };
  return {
    operation,
    status: "conflict",
    rebased: operation,
    current: { expect: block.expect, text: blockText(block) },
  };
}

// Block identities are positions, so a conflict shows the block now at that
// position: keeping mine applies the operation to it.
export function reconcile(
  head: DocumentLayout,
  operations: DocOperation[],
): Reconciliation<DocOperation>[] {
  return operations.map((operation) => {
    if (operation.type === "replaceText") return reconcileText(head, operation);
    if (operation.type === "insertParagraph")
      return operation.after === undefined
        ? { operation, status: "clean", rebased: operation }
        : reconcileBlock(head, operation, operation.after, operation.expect!);
    return reconcileBlock(head, operation, operation.id, operation.expect);
  });
}

export function withCurrent(
  operation: DocOperation,
  current: unknown,
): DocOperation {
  return operation.type === "replaceText"
    ? { ...operation, expect: current as string }
    : { ...operation, expect: (current as { expect: string }).expect };
}

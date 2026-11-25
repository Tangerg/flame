import { parseJSON, projectNumber } from "@frame/sdk/json";
import type { CellValue, DraftOperation } from "../contracts/content";
import type { SheetCellView, WorkbookLayout } from "../contracts/layouts";
import { edited, same, type Draft, type Reconciliation } from "./live-draft";

export type CellOperation = DraftOperation<"xlsx">;

export const cellId = (sheet: string, ref: string) => `${sheet}!${ref}`;

function split(id: string) {
  const separator = id.lastIndexOf("!");
  return { sheet: id.slice(0, separator), ref: id.slice(separator + 1) };
}

function cellOf(
  layout: WorkbookLayout,
  id: string,
): SheetCellView | undefined {
  const { sheet, ref } = split(id);
  return layout.sheets
    .find((entry) => entry.id === sheet)
    ?.cells.find((cell) => cell.ref === ref);
}

// An absent cell is blank and its setCell precondition is "".
const blank: CellValue = { type: "blank" };

// One user action is one undo step, however many cells it touches.
export function record(
  draft: Draft<CellOperation>,
  changes: { operation: CellOperation; original: SheetCellView | undefined }[],
): Draft<CellOperation> {
  let operations = draft.operations;
  for (const { operation, original } of changes) {
    const prior = operations.find((entry) => entry.id === operation.id);
    const merged = prior ? { ...operation, expect: prior.expect } : operation;
    const remaining = operations.filter((entry) => entry !== prior);
    const unchanged = same(merged.value, original ? original.input : blank);
    operations = unchanged ? remaining : [...remaining, merged];
  }
  return same(operations, draft.operations) ? draft : edited(draft, operations);
}

export function mergeSuggestion(
  draft: Draft<CellOperation>,
  layout: WorkbookLayout,
  offered: CellOperation[],
): Draft<CellOperation> {
  return record(
    draft,
    offered.map((operation) => ({
      operation,
      original: cellOf(layout, operation.id),
    })),
  );
}

// Typed text follows spreadsheet entry: "=" starts a formula, a leading
// apostrophe keeps the rest as text, and numbers and booleans are typed. A
// number JavaScript cannot hold exactly, or one written with leading zeros,
// stays text so its digits are never rounded.
export function parseEntry(text: string): CellValue {
  if (text === "") return blank;
  if (text.startsWith("'")) return { type: "string", value: text.slice(1) };
  if (text.startsWith("=") && text.length > 1)
    return { type: "formula", value: text };
  if (/^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$/u.test(text)) {
    try {
      return { type: "number", value: projectNumber(parseJSON(text)) };
    } catch (error) {
      if (!(error instanceof RangeError)) throw error;
    }
  }
  if (/^(?:true|false)$/iu.test(text))
    return { type: "boolean", value: text.toLowerCase() === "true" };
  return { type: "string", value: text };
}

export function entryText(value: CellValue | undefined): string {
  switch (value?.type) {
    case undefined:
    case "blank":
      return "";
    case "formula":
      return value.value;
    case "boolean":
      return value.value ? "TRUE" : "FALSE";
    case "number":
      return String(value.value);
    case "string":
      return parseEntry(value.value).type === "string"
        ? value.value
        : `'${value.value}`;
  }
}

export function reconcile(
  head: WorkbookLayout,
  operations: CellOperation[],
): Reconciliation<CellOperation>[] {
  return operations.map((operation): Reconciliation<CellOperation> => {
    const { sheet } = split(operation.id);
    if (!head.sheets.some((entry) => entry.id === sheet))
      return { operation, status: "missing" };
    const current = cellOf(head, operation.id);
    const expect = current?.expect ?? "";
    if (same(current ? current.input : blank, operation.value))
      return { operation, status: "applied", rebased: operation };
    if (expect === operation.expect)
      return { operation, status: "clean", rebased: operation };
    return {
      operation,
      status: "conflict",
      rebased: operation,
      current: { expect, input: current?.input },
    };
  });
}

export function withCurrent(
  operation: CellOperation,
  current: unknown,
): CellOperation {
  return { ...operation, expect: (current as { expect: string }).expect };
}

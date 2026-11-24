import { parseJSON, projectNumber, projectNumbers } from "@frame/sdk/json";
import {
  cellValueSchema,
  type Inspection,
  type CellValue,
} from "../contracts/content";
import type { DeliverablesState } from "./useDeliverables";

// The part of the deliverables state this view reads.
export type AuthoringState = Pick<
  DeliverablesState,
  "blocked" | "run" | "selectRevision" | "selection" | "signal"
>;

export function cell(text: string): CellValue {
  if (text.startsWith("=")) return { type: "formula", value: text };
  if (text === "") return { type: "blank" };
  if (/^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$/u.test(text))
    return { type: "number", value: projectNumber(parseJSON(text)) };
  return { type: "string", value: text };
}

export function columnName(index: number): string {
  let name = "";
  for (let value = index + 1; value; value = Math.floor((value - 1) / 26))
    name = String.fromCharCode(65 + ((value - 1) % 26)) + name;
  return name;
}

export type EditMode = "item" | "append" | "advanced";

export type InspectionItem = Inspection["items"][number];

export const EDITABLE_KINDS = new Set(["text", "cell", "page", "field"]);

export interface ItemDraft {
  text: string;
  valueType: CellValue["type"];
  degrees: string;
}

// A cell or field item's text is its JSON value; the draft edits the value.
export function itemDraftText(
  item: InspectionItem,
): Pick<ItemDraft, "text"> & Partial<Pick<ItemDraft, "valueType">> {
  if (item.kind === "cell") {
    const parsed = cellValueSchema.safeParse(JSON.parse(item.text));
    if (!parsed.success) return { valueType: "string", text: item.text };
    return {
      valueType: parsed.data.type,
      text: "value" in parsed.data ? String(parsed.data.value) : "",
    };
  }
  if (item.kind === "field") {
    const source = JSON.parse(item.text) as { value?: unknown };
    return { text: JSON.stringify(source.value ?? "") };
  }
  return { text: item.text };
}

function cellDraftValue({ valueType, text }: ItemDraft): unknown {
  if (valueType === "blank") return { type: "blank" };
  if (valueType === "number")
    return { type: valueType, value: projectNumber(parseJSON(text.trim())) };
  if (valueType === "boolean")
    return { type: valueType, value: text === "true" };
  return { type: valueType, value: text };
}

export function itemOperation(item: InspectionItem, draft: ItemDraft): unknown {
  const target = { id: item.id, expect: item.text };
  switch (item.kind) {
    case "cell":
      return {
        type: "setCell",
        ...target,
        value: cellValueSchema.parse(cellDraftValue(draft)),
      };
    case "page":
      return { type: "rotatePage", ...target, degrees: Number(draft.degrees) };
    case "field":
      return {
        type: "fillField",
        ...target,
        value: projectNumbers(parseJSON(draft.text)),
      };
    default:
      return { type: "replaceText", ...target, text: draft.text };
  }
}

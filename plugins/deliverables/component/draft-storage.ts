import { z } from "zod/v4";
import {
  documentEditSchema,
  presentationEditSchema,
  workbookEditSchema,
} from "../contracts/content";
import type { Draft } from "./live-draft";

const EDITS = {
  pptx: presentationEditSchema,
  xlsx: workbookEditSchema,
  docx: documentEditSchema,
};
export type DraftFormat = keyof typeof EDITS;

function draftSchema(format: DraftFormat) {
  const operation = EDITS[format].shape.operations.element;
  return z.strictObject({
    base: z.string().regex(/^[a-f0-9]{64}$/u),
    operations: z.array(operation).min(1),
    undo: z.array(z.array(operation)),
  });
}

// Unsaved drafts stay in this browser, so a reload or a later visit finds
// them. Each is checked against its format's edit vocabulary when read back;
// anything else is not a draft of this editor.
export function readDrafts<Operation>(
  key: string,
  format: DraftFormat,
): Record<string, Draft<Operation>> {
  let stored: unknown;
  try {
    stored = JSON.parse(localStorage.getItem(key) ?? "{}");
  } catch (error) {
    if (!(error instanceof SyntaxError)) throw error;
    return {};
  }
  if (!stored || typeof stored !== "object" || Array.isArray(stored)) return {};
  const schema = draftSchema(format);
  return Object.fromEntries(
    Object.entries(stored).flatMap(([id, value]) => {
      const parsed = schema.safeParse(value);
      return parsed.success ? [[id, parsed.data as Draft<Operation>]] : [];
    }),
  );
}

export function writeDrafts<Operation>(
  key: string,
  drafts: Record<string, Draft<Operation>>,
): void {
  const kept = Object.entries(drafts).filter(
    ([, draft]) => draft.operations.length,
  );
  try {
    if (kept.length)
      localStorage.setItem(key, JSON.stringify(Object.fromEntries(kept)));
    else localStorage.removeItem(key);
  } catch (error) {
    // A full or unavailable storage keeps the draft in this page only; the
    // draft itself is unaffected.
    if (!(error instanceof DOMException)) throw error;
  }
}

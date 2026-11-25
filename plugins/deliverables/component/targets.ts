import { z } from "zod/v4";

// What a deliverables link addresses. Without a revision it follows the head;
// the object is the identity the document's own editor uses: a slide or
// element, "sheet!A1" for a cell, or a block.
const id = z.string().regex(/^[a-f0-9]{64}$/u);
const targetSchema = z.strictObject({
  document: id,
  revision: id.optional(),
  object: z.string().min(1).max(2048).optional(),
});
export type ViewTarget = z.infer<typeof targetSchema>;

export const DELIVERABLES_VIEW = "deliverables";

export const encodeTarget = (target: ViewTarget): string =>
  JSON.stringify(target);

// A link is user input: anything else is not a target of this view.
export function parseTarget(value: string): ViewTarget | undefined {
  try {
    return targetSchema.parse(JSON.parse(value));
  } catch {
    return undefined;
  }
}

// A target's object for the editor to locate in the revision the link opened;
// the key distinguishes repeated openings of the same object.
export type Focus = { object?: string; revision: string; key: number };

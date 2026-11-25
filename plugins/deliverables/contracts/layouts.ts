import { z } from "zod/v4";
import {
  CHART_TYPES,
  cellAddressSchema,
  cellRangeSchema,
  cellValueSchema,
  chartDataSchema,
  extentSchema,
  frameSchema,
} from "./content";

// What the live editors read: projections derived from one native revision.
const textRunSchema = z.strictObject({
  id: z.string().min(1).max(1024),
  text: z.string(),
});
export const slideLayoutSchema = z.strictObject({
  width: extentSchema,
  height: extentSchema,
  slides: z.array(
    z.strictObject({
      id: z.string().min(1).max(1024),
      number: z.number().int().min(1),
      name: z.string(),
      hidden: z.boolean(),
      elements: z.array(
        z.strictObject({
          id: z.string().min(1).max(1024),
          kind: z.enum([
            "text",
            "shape",
            "picture",
            "chart",
            "table",
            "group",
            "connector",
            "other",
          ]),
          name: z.string(),
          frame: frameSchema.optional(),
          frameSource: z.enum(["slide", "layout", "master"]).optional(),
          rotation: z.number().finite().optional(),
          text: z.array(textRunSchema),
          chart: z
            .strictObject({
              type: z.enum(CHART_TYPES),
              // "chart": the embedded workbook holds only these ranges and is
              // rewritten with the data; "mixed": it holds more, so edits are refused.
              workbook: z.enum(["none", "chart", "mixed"]),
              data: chartDataSchema,
            })
            .optional(),
        }),
      ),
      notes: z.array(textRunSchema),
    }),
  ),
});
export type SlideLayout = z.infer<typeof slideLayoutSchema>;

// What a cell shows. A formula cell shows its calculated result, blank until
// recalculation; a number JSON cannot carry exactly keeps its native decimal text.
const shownCellSchema = z.union([
  z.strictObject({ type: z.literal("string"), value: z.string() }),
  z.strictObject({ type: z.literal("number"), value: z.number().finite() }),
  z.strictObject({
    type: z.literal("number"),
    value: z.string(),
    representation: z.literal("native_decimal"),
  }),
  z.strictObject({ type: z.literal("boolean"), value: z.boolean() }),
  z.strictObject({ type: z.literal("error"), value: z.string() }),
  z.strictObject({ type: z.literal("blank") }),
]);
export type ShownCell = z.infer<typeof shownCellSchema>;
export const workbookLayoutSchema = z.strictObject({
  sheets: z.array(
    z.strictObject({
      id: z.string().min(1).max(1024),
      name: z.string(),
      rows: z.number().int().min(0),
      columns: z.number().int().min(0),
      frozen: z.strictObject({
        rows: z.number().int().min(0),
        columns: z.number().int().min(0),
      }),
      // setCell refuses these cells; a merged range stays editable at its anchor.
      locked: z.array(
        z.strictObject({
          range: cellRangeSchema,
          reason: z.enum(["merge", "table", "formulaRegion"]),
          anchor: cellAddressSchema.optional(),
        }),
      ),
      cells: z.array(
        z.strictObject({
          ref: cellAddressSchema,
          // The exact setCell precondition; an absent cell's is "".
          expect: z.string(),
          shown: shownCellSchema,
          // The setCell value that reproduces the cell, when one exists.
          input: cellValueSchema.optional(),
        }),
      ),
    }),
  ),
});
export type WorkbookLayout = z.infer<typeof workbookLayoutSchema>;
export type SheetView = WorkbookLayout["sheets"][number];
export type SheetCellView = SheetView["cells"][number];
export type SlideView = SlideLayout["slides"][number];
export type ElementView = SlideView["elements"][number];

const blockIdentity = {
  id: z.string().min(1).max(1024),
  // SHA-256 of the block's exact XML: the precondition for block operations.
  expect: z.string().regex(/^[0-9a-f]{64}$/u),
};
export const documentLayoutSchema = z.strictObject({
  blocks: z.array(
    z.discriminatedUnion("kind", [
      z.strictObject({
        kind: z.literal("paragraph"),
        ...blockIdentity,
        // The paragraph style id; "" when the paragraph names none.
        style: z.string(),
        list: z.boolean(),
        alignment: z.string().optional(),
        runs: z.array(
          z.strictObject({
            id: z.string().min(1).max(1024),
            text: z.string(),
            bold: z.boolean(),
            italic: z.boolean(),
            underline: z.boolean(),
          }),
        ),
      }),
      z.strictObject({
        kind: z.literal("table"),
        ...blockIdentity,
        rows: z.array(z.array(z.string())),
      }),
      z.strictObject({
        kind: z.literal("other"),
        ...blockIdentity,
        element: z.string(),
        text: z.string(),
      }),
    ]),
  ),
});
export type DocumentLayout = z.infer<typeof documentLayoutSchema>;
export type BlockView = DocumentLayout["blocks"][number];
export type ParagraphView = Extract<BlockView, { kind: "paragraph" }>;

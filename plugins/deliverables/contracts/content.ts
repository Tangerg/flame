import { z } from "zod/v4";
import { parseJSON } from "@qimu/sdk/json";
import { LIMITS } from "./limits";
import { checkSchema } from "./evidence";
import { projectNumbers } from "./json";

export const contentLimits = {
  fileBytes: LIMITS.fileBytes,
  expandedBytes: 128 * 1024 * 1024,
  archiveParts: 10_000,
  xmlBytes: 16 * 1024 * 1024,
  xmlNodes: 250_000,
  jsonBytes: LIMITS.inputBytes,
  cells: 50_000,
  items: 1000,
  itemText: 4096,
  inspectionBytes: LIMITS.reportBytes,
} as const;

export const formatSchema = z.enum(["docx", "xlsx", "pptx", "pdf"]);
export type Format = z.infer<typeof formatSchema>;
export type OfficeFormat = Exclude<Format, "pdf">;
export const inspectionSchema = z.strictObject({
  format: formatSchema,
  title: z.string().max(256).optional(),
  items: z
    .array(
      z.strictObject({
        id: z.string(),
        kind: z.string(),
        text: z.string().max(contentLimits.itemText),
      }),
    )
    .max(contentLimits.items),
  facts: z.record(z.string(), z.union([z.string(), z.number(), z.boolean()])),
  checks: z.array(checkSchema),
  truncated: z.boolean(),
});
export type Inspection = z.infer<typeof inspectionSchema>;
export const formatCatalogSchema = z.strictObject({
  formats: z.array(
    z.strictObject({
      format: formatSchema,
      createSchemaJSON: z.string().optional(),
      editSchemaJSON: z.string(),
      exampleJSON: z.string().optional(),
      limitations: z.array(z.string()),
    }),
  ),
});
export type FormatCatalog = z.infer<typeof formatCatalogSchema>;

const text = z.string().max(100_000);
const shortText = z.string().max(4096);
const color = z.string().regex(/^[0-9a-fA-F]{6}$/u);
const positive = z.number().finite().positive();
const url = z
  .string()
  .max(2048)
  .refine((value) => {
    try {
      const parsed = new URL(value);
      return (
        ["https:", "http:", "mailto:"].includes(parsed.protocol) &&
        parsed.username === "" &&
        parsed.password === ""
      );
    } catch {
      return false;
    }
  }, "expected an http, https or mailto URL without credentials");

export const imageSchema = z.strictObject({
  mime: z.enum(["image/png", "image/jpeg"]),
  base64: z.string().max(12 * 1024 * 1024),
  alt: shortText,
});
export type AuthoredImage = z.infer<typeof imageSchema>;

const runSchema = z.strictObject({
  text,
  bold: z.boolean().optional(),
  italic: z.boolean().optional(),
  underline: z.boolean().optional(),
  color: color.optional(),
  size: positive.max(144).optional(),
  url: url.optional(),
});
const paragraphSchema = z.strictObject({
  type: z.literal("paragraph"),
  runs: z.array(runSchema).min(1).max(1000),
  heading: z.number().int().min(1).max(6).optional(),
  alignment: z.enum(["left", "center", "right", "justify"]).optional(),
  bullet: z.boolean().optional(),
  keepNext: z.boolean().optional(),
});
const docBlockSchema = z.discriminatedUnion("type", [
  paragraphSchema,
  z.strictObject({
    type: z.literal("table"),
    headers: z.array(shortText).min(1).max(32),
    rows: z.array(z.array(shortText).max(32)).max(2000),
  }),
  z.strictObject({
    type: z.literal("image"),
    image: imageSchema,
    width: positive.max(1500),
    height: positive.max(1500),
    caption: shortText.optional(),
  }),
  z.strictObject({ type: z.literal("pageBreak") }),
]);
export const documentContentSchema = z.strictObject({
  title: z.string().min(1).max(256),
  author: z.string().max(256).optional(),
  language: z.string().max(32).optional(),
  font: z.string().min(1).max(100).optional(),
  fontSize: positive.max(72).optional(),
  accent: color.optional(),
  header: shortText.optional(),
  footer: shortText.optional(),
  pageNumbers: z.boolean().optional(),
  landscape: z.boolean().optional(),
  blocks: z.array(docBlockSchema).min(1).max(2000),
  references: z
    .array(
      z.strictObject({
        id: z.string().min(1).max(64),
        title: shortText,
        url: url.optional(),
        accessed: z.string().max(64).optional(),
      }),
    )
    .max(1000)
    .optional(),
});
export type DocumentContent = z.infer<typeof documentContentSchema>;

export const cellValueSchema = z.discriminatedUnion("type", [
  z.strictObject({ type: z.literal("string"), value: shortText }),
  z.strictObject({ type: z.literal("number"), value: z.number().finite() }),
  z.strictObject({ type: z.literal("boolean"), value: z.boolean() }),
  z.strictObject({
    type: z.literal("formula"),
    value: z.string().min(2).max(8192).startsWith("="),
  }),
  z.strictObject({ type: z.literal("blank") }),
]);
export type CellValue = z.infer<typeof cellValueSchema>;
const address = z.string().regex(/^[A-Z]{1,3}[1-9][0-9]{0,6}$/u);
const range = z
  .string()
  .regex(/^[A-Z]{1,3}[1-9][0-9]{0,6}:[A-Z]{1,3}[1-9][0-9]{0,6}$/u);
export const cellStyleSchema = z.strictObject({
  bold: z.boolean().optional(),
  italic: z.boolean().optional(),
  fontSize: positive.max(144).optional(),
  color: color.optional(),
  fill: color.optional(),
  numberFormat: z.string().max(256).optional(),
  horizontal: z.enum(["left", "center", "right"]).optional(),
  wrap: z.boolean().optional(),
  border: z.boolean().optional(),
});
export const workbookChartSchema = z.strictObject({
  type: z.enum(["bar", "line", "pie"]),
  title: shortText,
  categories: range,
  series: z
    .array(
      z.strictObject({
        name: shortText,
        values: range,
        color: color.optional(),
      }),
    )
    .min(1)
    .max(20),
  anchor: address,
  width: positive.max(2000).optional(),
  height: positive.max(2000).optional(),
});
export type WorkbookChart = z.infer<typeof workbookChartSchema>;
export const workbookContentSchema = z.strictObject({
  title: z.string().min(1).max(256),
  author: z.string().max(256).optional(),
  sheets: z
    .array(
      z.strictObject({
        name: z
          .string()
          .min(1)
          .max(31)
          .regex(/^[^\\/*?:[\]]+$/u),
        cells: z
          .array(
            z.strictObject({
              address,
              value: cellValueSchema,
              style: cellStyleSchema.optional(),
              note: shortText.optional(),
            }),
          )
          .max(contentLimits.cells),
        columns: z
          .array(
            z.strictObject({
              column: z.number().int().min(1).max(16384),
              width: positive.max(255),
            }),
          )
          .max(256)
          .optional(),
        freezeRows: z.number().int().min(0).max(1000).optional(),
        freezeColumns: z.number().int().min(0).max(100).optional(),
        autoFilter: range.optional(),
        merges: z.array(range).max(1000).optional(),
        tables: z
          .array(
            z.strictObject({
              name: z.string().regex(/^[A-Za-z_][A-Za-z0-9_]{0,63}$/u),
              range,
            }),
          )
          .max(100)
          .optional(),
        charts: z.array(workbookChartSchema).max(50).optional(),
        landscape: z.boolean().optional(),
      }),
    )
    .min(1)
    .max(50),
});
export type WorkbookContent = z.infer<typeof workbookContentSchema>;

const position = {
  x: z.number().min(0).max(40),
  y: z.number().min(0).max(40),
  width: positive.max(40),
  height: positive.max(40),
};
const slideElementSchema = z.discriminatedUnion("type", [
  z.strictObject({
    type: z.literal("text"),
    ...position,
    text,
    fontSize: positive.max(144).optional(),
    bold: z.boolean().optional(),
    italic: z.boolean().optional(),
    color: color.optional(),
    align: z.enum(["left", "center", "right"]).optional(),
    bullet: z.boolean().optional(),
    url: url.optional(),
  }),
  z.strictObject({
    type: z.literal("shape"),
    ...position,
    shape: z.enum([
      "rect",
      "roundRect",
      "ellipse",
      "line",
      "chevron",
      "triangle",
    ]),
    fill: color.optional(),
    line: color.optional(),
  }),
  z.strictObject({ type: z.literal("image"), ...position, image: imageSchema }),
  z.strictObject({
    type: z.literal("table"),
    ...position,
    rows: z.array(z.array(shortText).min(1).max(20)).min(1).max(100),
    fontSize: positive.max(72).optional(),
    header: z.boolean().optional(),
  }),
  z.strictObject({
    type: z.literal("chart"),
    ...position,
    chart: z.enum(["bar", "line", "pie", "doughnut", "area", "scatter"]),
    title: shortText.optional(),
    series: z
      .array(
        z.strictObject({
          name: shortText,
          labels: z.array(shortText).min(1).max(1000),
          values: z.array(z.number().finite()).min(1).max(1000),
        }),
      )
      .min(1)
      .max(20),
  }),
]);
export type SlideElement = z.infer<typeof slideElementSchema>;
export const presentationContentSchema = z.strictObject({
  title: z.string().min(1).max(256),
  author: z.string().max(256).optional(),
  subject: z.string().max(1000).optional(),
  language: z.string().max(32).optional(),
  font: z.string().min(1).max(100).optional(),
  layout: z.enum(["wide", "standard"]).optional(),
  masters: z
    .array(
      z.strictObject({
        name: z.string().min(1).max(100),
        background: color.optional(),
        elements: z
          .array(
            z.union([
              slideElementSchema.options[0],
              slideElementSchema.options[1].extend({
                shape: z.enum(["rect", "line"]),
              }),
              slideElementSchema.options[2],
            ]),
          )
          .max(100),
      }),
    )
    .max(20)
    .optional(),
  slides: z
    .array(
      z.strictObject({
        title: z.string().max(256).optional(),
        master: z.string().min(1).max(100).optional(),
        background: color.optional(),
        elements: z.array(slideElementSchema).max(200),
        notes: text.optional(),
      }),
    )
    .min(1)
    .max(200),
});
export type PresentationContent = z.infer<typeof presentationContentSchema>;

const replaceText = z.strictObject({
  type: z.literal("replaceText"),
  id: z.string().min(1).max(1024),
  expect: text,
  text,
});
const reorderSlides = z.strictObject({
  type: z.literal("reorderSlides"),
  ids: z.array(z.string().min(1).max(1024)).min(1).max(2000),
});
export const documentEditSchema = z.strictObject({
  operations: z
    .array(
      z.discriminatedUnion("type", [
        replaceText,
        z.strictObject({
          type: z.literal("appendParagraph"),
          text,
          style: z
            .enum(["Normal", "Heading1", "Heading2", "Heading3"])
            .optional(),
        }),
      ]),
    )
    .min(1)
    .max(1000),
});
export const workbookEditSchema = z.strictObject({
  operations: z
    .array(
      z.strictObject({
        type: z.literal("setCell"),
        id: z.string().min(1).max(1024),
        expect: text,
        value: cellValueSchema,
      }),
    )
    .min(1)
    .max(1000),
});
export const presentationEditSchema = z.strictObject({
  operations: z
    .array(z.discriminatedUnion("type", [replaceText, reorderSlides]))
    .min(1)
    .max(1000),
});
export const pdfEditSchema = z.strictObject({
  operations: z
    .array(
      z.discriminatedUnion("type", [
        z.strictObject({
          type: z.literal("rotatePage"),
          id: z.string().regex(/^page:[1-9][0-9]*$/u),
          expect: shortText,
          degrees: z.union([
            z.literal(0),
            z.literal(90),
            z.literal(180),
            z.literal(270),
          ]),
        }),
        z.strictObject({
          type: z.literal("reorderPages"),
          ids: z
            .array(z.string().regex(/^page:[1-9][0-9]*$/u))
            .min(1)
            .max(2000),
        }),
        z.strictObject({
          type: z.literal("fillField"),
          id: z.string().startsWith("field:").max(2048),
          expect: shortText,
          value: z.union([
            shortText,
            z.boolean(),
            z.array(shortText).max(1000),
          ]),
        }),
      ]),
    )
    .min(1)
    .max(1000),
});

export function parseContent<T>(schema: z.ZodType<T>, json: string): T {
  if (new TextEncoder().encode(json).byteLength > contentLimits.jsonBytes)
    throw new Error("content exceeds the JSON byte limit");
  return schema.parse(projectNumbers(parseJSON(json)));
}

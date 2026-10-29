import { z } from "zod/v4";
import {
  documentContentSchema,
  documentEditSchema,
  pdfEditSchema,
  presentationContentSchema,
  presentationEditSchema,
  workbookContentSchema,
  workbookEditSchema,
  contentLimits,
  type Format,
  type FormatCatalog,
  type Inspection,
  type OfficeFormat,
} from "../../contracts/content";
import { OfficePackage } from "./archive";
import { createDocument, editDocument, inspectDocument } from "./document";
import { editPDF, inspectPDF } from "./pdf";
import {
  createPresentation,
  editPresentation,
  inspectPresentation,
} from "./presentation";
import {
  createWorkbook,
  editWorkbook,
  inspectWorkbook,
  workbookCSV,
} from "./workbook";

export { encodeArchive } from "./archive";
export type { Format, OfficeFormat, Inspection } from "../../contracts/content";

export async function createFile(
  format: OfficeFormat,
  contentJSON: string,
): Promise<Uint8Array> {
  switch (format) {
    case "docx":
      return createDocument(contentJSON);
    case "xlsx":
      return createWorkbook(contentJSON);
    case "pptx":
      return createPresentation(contentJSON);
    default:
      throw new Error("unsupported native creation format");
  }
}

export async function inspectFile(
  format: Format,
  bytes: Uint8Array,
): Promise<Inspection> {
  if (format === "pdf") return inspectPDF(bytes);
  const archive = OfficePackage.open(format, bytes);
  switch (format) {
    case "docx":
      return inspectDocument(archive);
    case "xlsx":
      return inspectWorkbook(archive);
    case "pptx":
      return inspectPresentation(archive);
    default:
      throw new Error("unsupported native inspection format");
  }
}

export async function editFile(
  format: Format,
  bytes: Uint8Array,
  operationsJSON: string,
): Promise<Uint8Array> {
  if (format === "pdf") return editPDF(bytes, operationsJSON);
  const archive = OfficePackage.open(format, bytes);
  let result: Uint8Array;
  switch (format) {
    case "docx":
      result = editDocument(archive, operationsJSON);
      break;
    case "xlsx":
      result = editWorkbook(archive, operationsJSON);
      break;
    case "pptx":
      result = editPresentation(archive, operationsJSON);
      break;
    default:
      throw new Error("unsupported native editing format");
  }
  OfficePackage.open(format, result);
  return result;
}

export function exportCSV(bytes: Uint8Array, sheetPart: string): Uint8Array {
  const result = Buffer.from(
    workbookCSV(OfficePackage.open("xlsx", bytes), sheetPart),
    "utf8",
  );
  if (result.length > contentLimits.fileBytes)
    throw new Error("CSV export exceeds the native file byte limit");
  return result;
}

const officeLimits = [
  "Transitional, macro-free OOXML packages with canonical main parts are supported; strict OOXML, encryption, ZIP64, macros, active embedded content, external data and unsupported embedded files are refused.",
  "Creation JSON is command input. Native revision bytes are the editable source of truth; imported files are never reconstructed from inspection text.",
  "Native edits preserve untouched archive entries byte-for-byte after decoding. Recompression and selected XML part serialization can change the ZIP bytes.",
  "Inspection text runs longer than 4096 characters are omitted; reports are bounded to 1000 items and 1 MiB. IDs apply only to the exact source revision.",
  "Rendering and actual workbook recalculation require the configured isolated LibreOffice engine. Structural admission does not certify layout, content truth or fresh formula results.",
];
const examples = {
  docx: {
    title: "Quarterly operating review",
    author: "Operations",
    header: "Operating review",
    pageNumbers: true,
    blocks: [
      {
        type: "paragraph",
        heading: 1,
        runs: [{ text: "Quarterly operating review" }],
      },
      {
        type: "paragraph",
        runs: [
          { text: "Revenue grew 12% against the prior quarter. [finance]" },
        ],
      },
      {
        type: "table",
        headers: ["Metric", "Value", "Unit"],
        rows: [
          ["Revenue", "112", "USD million"],
          ["Growth", "12", "%"],
        ],
      },
    ],
    references: [{ id: "finance", title: "Finance close, Q2" }],
  },
  xlsx: {
    title: "Revenue model",
    sheets: [
      {
        name: "Revenue",
        freezeRows: 1,
        columns: [
          { column: 1, width: 20 },
          { column: 2, width: 18 },
        ],
        cells: [
          {
            address: "A1",
            value: { type: "string", value: "Quarter" },
            style: { bold: true },
          },
          {
            address: "B1",
            value: { type: "string", value: "Revenue (USD million)" },
            style: { bold: true, wrap: true },
          },
          { address: "A2", value: { type: "string", value: "Q1" } },
          { address: "B2", value: { type: "number", value: 100 } },
          { address: "A3", value: { type: "string", value: "Q2" } },
          { address: "B3", value: { type: "number", value: 112 } },
          { address: "A4", value: { type: "string", value: "Total" } },
          { address: "B4", value: { type: "formula", value: "=SUM(B2:B3)" } },
        ],
        tables: [{ name: "RevenueData", range: "A1:B3" }],
        charts: [
          {
            type: "bar",
            title: "Quarterly revenue",
            categories: "A2:A3",
            series: [{ name: "Revenue", values: "B2:B3" }],
            anchor: "D2",
          },
        ],
      },
    ],
  },
  pptx: {
    title: "Quarterly operating review",
    layout: "wide",
    masters: [
      {
        name: "Review",
        background: "F7FAFC",
        elements: [
          {
            type: "text",
            x: 0.5,
            y: 7,
            width: 12,
            height: 0.25,
            text: "Operations · Quarterly review",
            fontSize: 10,
          },
        ],
      },
    ],
    slides: [
      {
        title: "Revenue increased 12%",
        master: "Review",
        elements: [
          {
            type: "text",
            x: 0.6,
            y: 0.5,
            width: 12,
            height: 0.8,
            text: "Revenue increased 12%",
            fontSize: 32,
            bold: true,
          },
          {
            type: "chart",
            chart: "bar",
            x: 0.8,
            y: 1.8,
            width: 8,
            height: 4.5,
            series: [
              {
                name: "Revenue (USD million)",
                labels: ["Q1", "Q2"],
                values: [100, 112],
              },
            ],
          },
          {
            type: "text",
            x: 9.3,
            y: 2,
            width: 3.2,
            height: 2,
            text: "Units: USD million\nSource: Finance close",
            fontSize: 18,
          },
        ],
        notes:
          "Explain the quarter-over-quarter comparison and confirm that both periods use the same accounting basis.",
      },
    ],
  },
};

export function catalog(): FormatCatalog {
  const schema = (value: z.ZodType) =>
    JSON.stringify(z.toJSONSchema(value, { target: "draft-7" }));
  return {
    formats: [
      {
        format: "docx",
        createSchemaJSON: schema(documentContentSchema),
        editSchemaJSON: schema(documentEditSchema),
        exampleJSON: JSON.stringify(examples.docx),
        limitations: [
          ...officeLimits,
          "Creates rich runs, headings, lists, tables, PNG/JPEG images, headers, footers, page numbers and explicit references. Native edits replace exact text runs or append simple paragraphs; automatic pagination and bibliography verification are renderer/author concerns.",
        ],
      },
      {
        format: "xlsx",
        createSchemaJSON: schema(workbookContentSchema),
        editSchemaJSON: schema(workbookEditSchema),
        exampleJSON: JSON.stringify(examples.xlsx),
        limitations: [
          ...officeLimits,
          "Creates typed cells, local formulas, cell styles, comments, tables, merges, freeze panes and editable native bar/line/pie charts. At most 50000 authored cells, 100000 rows and 1024 authored columns.",
          "Excel numbers use floating-point arithmetic and approximately 15 decimal digits. Inputs that cannot be projected exactly are refused; use string cells for exact identifiers or decimal text, and explicit rounding/unit policies for numerical models. Imported numbers outside exact JavaScript projection are reported as native_decimal text; CSV preserves native numeric lexemes.",
          "Native setCell preserves cell style and unrelated parts. Shared/array/data-table formula regions cannot be edited. Every edit invalidates workbook formula results, chart caches and the calculation chain.",
          "Formula caches cannot be supplied during creation or editing. CSV export requires existing calculated results and refuses spreadsheet errors; formula-like string values receive an apostrophe to prevent spreadsheet formula injection.",
        ],
      },
      {
        format: "pptx",
        createSchemaJSON: schema(presentationContentSchema),
        editSchemaJSON: schema(presentationEditSchema),
        exampleJSON: JSON.stringify(examples.pptx),
        limitations: [
          ...officeLimits,
          "Creates editable text, shapes, tables, charts, PNG/JPEG images, speaker notes and masters. Masters support text, images, rectangles and lines. Slide measurements are in inches.",
          "Native edits replace exact text runs in slide/notes XML and reorder all original slides. Imported layouts, masters, chart workbooks and relationships remain native; arbitrary shape/chart/template restructuring is not supported.",
        ],
      },
      {
        format: "pdf",
        editSchemaJSON: schema(pdfEditSchema),
        limitations: [
          "PDF creation is the actual Office renderer output. Import supports passive, unencrypted PDFs with classic cross-reference tables; compressed object/cross-reference streams, signed files, embedded attachments, active actions, XFA and unsupported stream filters are refused before editing.",
          "Native operations rotate pages, reorder the complete original page set and fill supported AcroForm text, checkbox, choice and radio fields. Text body editing, OCR, redaction and signature preservation are not claimed.",
          "Form appearance regeneration uses the PDF library's supported standard font encoding; unsupported glyphs fail explicitly. Form fields stay editable and are not flattened.",
          "PDF inspection reports page geometry and form fields, not extracted body text. Actual page rendering/text extraction belongs to the configured isolated PDF engine.",
        ],
      },
    ],
  };
}

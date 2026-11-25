import { canonicalJSON, parseJSON } from "@flame/backend-sdk/mcp";
import { projectNumbers } from "@frame/sdk/json";
import { z } from "zod";
import {
  schemas,
  type Assessment,
  type Revision,
} from "../contracts/capability";
import { formatCatalogSchema, inspectionSchema } from "../contracts/content";
import {
  documentLayoutSchema,
  slideLayoutSchema,
  workbookLayoutSchema,
} from "../contracts/layouts";
import { renderReportSchema, reviewReportSchema } from "../contracts/evidence";
import { MODEL_LIMITS } from "../contracts/limits";
import type { Application, Operation } from "./application";
import type { Reply } from "./archive";
import { project, selectedPath, selection, size } from "./model-view";
import { refusal } from "./refusal";

// A model never names a Project; the Run's captured one scopes these calls.
export const modelInputs = {
  list: schemas.page,
  create: schemas.create.omit({ project: true }),
  import: schemas.import.omit({ project: true }),
  catalog: z.strictObject({
    format: schemas.format.optional(),
    section: z
      .enum(["overview", "create", "edit", "example", "engines", "limits"])
      .optional(),
    ...selection,
  }),
  read: schemas.readInput.extend({
    ...selection,
    sourceSha256: schemas.asset.shape.sha256
      .optional()
      .describe(
        "For an inspection path, supply the sourceSha256 returned with the selected revision; a mismatch is refused.",
      ),
  }),
  assessment: schemas.assessmentInput.extend(selection),
  slides: schemas.readInput.extend({
    ...selection,
    sourceSha256: schemas.asset.shape.sha256
      .optional()
      .describe(
        "For a layout path, supply the sourceSha256 returned with the selected revision; a mismatch is refused.",
      ),
  }),
  sheets: schemas.readInput.extend({
    ...selection,
    sourceSha256: schemas.asset.shape.sha256
      .optional()
      .describe(
        "For a sheet path, supply the sourceSha256 returned with the selected revision; a mismatch is refused.",
      ),
  }),
  blocks: schemas.readInput.extend({
    ...selection,
    sourceSha256: schemas.asset.shape.sha256
      .optional()
      .describe(
        "For a block path, supply the sourceSha256 returned with the selected revision; a mismatch is refused.",
      ),
  }),
};

export const modelDescriptions: Partial<Record<Operation, string>> = {
  catalog:
    "Read a short format/engine/limit overview first. Select format and section=create, edit or example for one exact authoring contract. section=engines or limits reads those facts. Large values support path/offset/limit; omitted entries remain retained, never inferred.",
  read: 'Read a native revision summary with its exact revision and sourceSha256. To inspect original content, supply both returned identities and path (for example ["inspection","items"] or ["revision","provenance"]). Paths address document, revision, inspection, processing and assessments. Document head and the append-ordered assessment list are current projections; native inspection and revision content are immutable. Append an entry\'s key to inspect it; continue at nextOffset with the same revision and hash. Text positions count Unicode code points. Retained inspection truncation is reported separately from projection paging.',
  slides:
    'Read a PPTX revision\'s slide outline with its exact revision and sourceSha256. To inspect slides, supply both identities and a path such as ["slides","0","elements"] or ["slides","0","notes"]. Elements carry ids for setFrame/deleteElement/setChartData, frames in EMUs (914400 per inch), text runs with replaceText ids, and chart data with workbook "none", "chart" (editable) or "mixed" (refused). Copy current values into expect exactly.',
  sheets:
    'Read an XLSX revision\'s sheet outline with its exact revision and sourceSha256. To inspect cells, supply both identities and a path such as ["sheets","0","cells"] or ["sheets","0","locked"]. A cell carries its ref, the exact setCell expect, its shown value (a formula\'s calculated result, blank until recalculation) and, when one exists, the setCell input that reproduces it. Address a cell for setCell as "<sheet id>!<ref>" and copy expect exactly; an absent cell\'s expect is "".',
  blocks:
    'Read a DOCX revision\'s body outline (block counts and headings with their block index) with its exact revision and sourceSha256. To inspect blocks, supply both identities and a path such as ["blocks"] or ["blocks","12"]. A paragraph carries its block id, expect, style ("" when none), list and alignment facts and its text runs with replaceText ids; tables carry cell text. Copy a block\'s expect exactly into insertParagraph (after), deleteBlock or setParagraphStyle, and a run\'s text into replaceText expect.',
  assessment:
    'Read an exact render/review summary. Use path=["report","pages"], ["report","checks"], ["report","extractedText"], ["report","notes"] or ["assets"] for original evidence. Append an entry\'s key to inspect it and continue at nextOffset. The page tool returns a real PNG for visual review.',
};

function revisionSummary(value: Revision) {
  return {
    id: value.id,
    documentId: value.documentId,
    ...(value.parent === undefined ? {} : { parent: value.parent }),
    createdAt: value.createdAt,
    operation: value.operation,
    ...(value.operation === "restore" ? { restored: value.restored } : {}),
    format: value.format,
    sha256: value.sha256,
    size: value.size,
    nativeAssetId: value.native.id,
  };
}

function assessmentSummary(value: Assessment) {
  const { reportJSON, assets, ...identity } = value;
  const report =
    value.kind === "render"
      ? renderReportSchema.parse(projectNumbers(parseJSON(reportJSON)))
      : reviewReportSchema.parse(projectNumbers(parseJSON(reportJSON)));
  return {
    ...identity,
    assetCount: assets.length,
    report:
      "verdict" in report
        ? {
            verdict: report.verdict,
            pages: report.pages,
            reviewer: report.reviewer,
          }
        : {
            pageCount: report.pages.length,
            fontStatus: report.fontStatus,
            checks: {
              error: report.checks.filter((check) => check.severity === "error")
                .length,
              warning: report.checks.filter(
                (check) => check.severity === "warning",
              ).length,
              info: report.checks.filter((check) => check.severity === "info")
                .length,
            },
          },
  };
}

export function commandProjection(operation: string, value: unknown): unknown {
  if (
    ["create", "edit", "import", "recalculate", "restore"].includes(operation)
  )
    return revisionSummary(schemas.revision.parse(value));
  if (operation === "render" || operation === "review")
    return assessmentSummary(schemas.assessment.parse(value));
  if (operation === "reply") {
    const thread = schemas.thread.parse(value);
    return {
      id: thread.id,
      status: thread.status,
      messages: thread.messages.length,
    };
  }
  return value;
}

type QuerySource = Pick<Application, "query">;

type QueryProjection = { retained: unknown; model: unknown };

type RetainedCatalog = ReturnType<typeof schemas.catalog.parse>;

type CatalogFormat = ReturnType<
  typeof formatCatalogSchema.parse
>["formats"][number];

type CatalogSection = NonNullable<
  ReturnType<typeof modelInputs.catalog.parse>["section"]
>;

// The overview may narrow to one format; engine and limit views never select
// one; schemas and examples always belong to exactly one format.
function catalogSource(
  section: CatalogSection,
  format: CatalogFormat | undefined,
  formats: readonly CatalogFormat[],
  retained: RetainedCatalog,
): unknown {
  if (section === "overview")
    return {
      formats: (format ? [format] : formats).map((format) => ({
        format: format.format,
        canCreate: format.createSchemaJSON !== undefined,
        limitations: format.limitations,
      })),
      engines: JSON.parse(retained.enginesJSON),
      limits: JSON.parse(retained.limitsJSON),
    };
  if (section === "engines" || section === "limits") {
    if (format)
      throw refusal(
        "invalid_projection",
        "engine and limit views do not select a format",
      );
    return parseJSON(
      section === "engines" ? retained.enginesJSON : retained.limitsJSON,
    );
  }
  if (!format)
    throw refusal(
      "invalid_projection",
      "select one format before reading its schema or example",
    );
  const original = {
    create: format.createSchemaJSON,
    edit: format.editSchemaJSON,
    example: format.exampleJSON,
  }[section];
  if (original === undefined)
    throw refusal(
      "unsupported_format_operation",
      "the selected format does not define this operation",
    );
  return parseJSON(original);
}

async function catalogProjection(
  application: QuerySource,
  input: Record<string, unknown>,
  signal: AbortSignal,
): Promise<QueryProjection> {
  const request = modelInputs.catalog.parse(input);
  const retained = schemas.catalog.parse(
    await application.query("catalog", {}, signal),
  );
  const formats = formatCatalogSchema.parse(
    JSON.parse(retained.formatsJSON),
  ).formats;
  const section = request.section ?? "overview";
  const format =
    request.format &&
    formats.find((format) => format.format === request.format);
  if (request.format && !format)
    throw refusal(
      "unsupported_format",
      "format is not in the original catalog",
    );
  const source = catalogSource(section, format, formats, retained);
  return {
    retained,
    model: {
      ...(request.format ? { format: request.format } : {}),
      section,
      view: project(source, request),
    },
  };
}

async function documentProjection(
  application: QuerySource,
  input: Record<string, unknown>,
  signal: AbortSignal,
): Promise<QueryProjection> {
  const request = modelInputs.read.parse(input);
  selectedPath(request);
  if (
    request.path !== undefined &&
    (request.revision === undefined || request.sourceSha256 === undefined)
  )
    throw refusal(
      "revision_required",
      "read a summary first, then pin its exact revision and sourceSha256 for inspection paging",
    );
  const retained = schemas.read.parse(
    await application.query(
      "read",
      { id: request.id, revision: request.revision },
      signal,
    ),
  );
  if (
    request.sourceSha256 !== undefined &&
    request.sourceSha256 !== retained.revision.sha256
  )
    throw refusal(
      "source_conflict",
      "the retained revision differs from the expected source SHA-256",
    );
  const inspection = inspectionSchema.parse(
    projectNumbers(parseJSON(retained.inspectionJSON)),
  );
  const source = {
    document: retained.document,
    revision: retained.revision,
    inspection,
    ...(retained.processingJSON === undefined
      ? {}
      : { processing: parseJSON(retained.processingJSON) }),
    assessments: retained.assessments,
  };
  const identity = {
    documentId: retained.document.id,
    revision: retained.revision.id,
    sourceSha256: retained.revision.sha256,
  };
  return {
    retained,
    model: {
      ...identity,
      ...(request.path === undefined
        ? {
            document: retained.document,
            native: revisionSummary(retained.revision),
            inspection: {
              format: inspection.format,
              ...(inspection.title === undefined
                ? {}
                : { title: inspection.title }),
              retainedTruncated: inspection.truncated,
              items: inspection.items.length,
              checks: inspection.checks.length,
              facts: Object.keys(inspection.facts).length,
            },
            assessmentCount: retained.assessments.length,
            processingAvailable: retained.processingJSON !== undefined,
          }
        : {
            view: project(source, request),
            retainedTruncated: inspection.truncated,
          }),
    },
  };
}

async function assessmentProjection(
  application: QuerySource,
  input: Record<string, unknown>,
  signal: AbortSignal,
): Promise<QueryProjection> {
  const request = modelInputs.assessment.parse(input);
  selectedPath(request);
  const retained = schemas.assessment.parse(
    await application.query(
      "assessment",
      {
        id: request.id,
        revision: request.revision,
        assessmentId: request.assessmentId,
      },
      signal,
    ),
  );
  const { reportJSON, ...metadata } = retained;
  return {
    retained,
    model: {
      documentId: request.id,
      ...(request.path === undefined
        ? assessmentSummary(retained)
        : {
            assessmentId: retained.id,
            revision: retained.revision,
            sourceSha256: retained.sourceSha256,
            view: project(
              { ...metadata, report: parseJSON(reportJSON) },
              request,
            ),
          }),
    },
  };
}

type LayoutOperation = Extract<Operation, "slides" | "sheets" | "blocks">;

// A model reads an outline first, then pages one exact revision's layout by path.
async function layoutProjection<T>(
  application: QuerySource,
  operation: LayoutOperation,
  input: Record<string, unknown>,
  signal: AbortSignal,
  schema: z.ZodType<T>,
  outline: (layout: T) => Record<string, unknown>,
): Promise<QueryProjection> {
  const request = modelInputs[operation].parse(input);
  selectedPath(request);
  if (
    request.path !== undefined &&
    (request.revision === undefined || request.sourceSha256 === undefined)
  )
    throw refusal(
      "revision_required",
      "read an outline first, then pin its exact revision and sourceSha256 for layout paging",
    );
  const retained = schemas[operation].parse(
    await application.query(
      operation,
      { id: request.id, revision: request.revision },
      signal,
    ),
  );
  if (
    request.sourceSha256 !== undefined &&
    request.sourceSha256 !== retained.sourceSha256
  )
    throw refusal(
      "source_conflict",
      "the retained revision differs from the expected source SHA-256",
    );
  const layout = schema.parse(projectNumbers(parseJSON(retained.layoutJSON)));
  return {
    retained,
    model: {
      documentId: retained.document.id,
      revision: retained.revision,
      sourceSha256: retained.sourceSha256,
      ...(request.path === undefined
        ? outline(layout)
        : { view: project(layout, request) }),
    },
  };
}

export async function queryProjection(
  application: QuerySource,
  operation: Operation,
  input: Record<string, unknown>,
  signal: AbortSignal,
): Promise<QueryProjection> {
  switch (operation) {
    case "catalog":
      return catalogProjection(application, input, signal);
    case "read":
      return documentProjection(application, input, signal);
    case "assessment":
      return assessmentProjection(application, input, signal);
    case "slides":
      return layoutProjection(
        application,
        operation,
        input,
        signal,
        slideLayoutSchema,
        (layout) => ({
          width: layout.width,
          height: layout.height,
          layouts: layout.layouts,
          slides: layout.slides.map((slide) => ({
            id: slide.id,
            number: slide.number,
            name: slide.name,
            elements: slide.elements.length,
            notes: slide.notes.length,
          })),
        }),
      );
    case "sheets":
      return layoutProjection(
        application,
        operation,
        input,
        signal,
        workbookLayoutSchema,
        (layout) => ({
          sheets: layout.sheets.map((sheet) => ({
            id: sheet.id,
            name: sheet.name,
            rows: sheet.rows,
            columns: sheet.columns,
            cells: sheet.cells.length,
            locked: sheet.locked.length,
          })),
        }),
      );
    case "blocks":
      return layoutProjection(
        application,
        operation,
        input,
        signal,
        documentLayoutSchema,
        (layout) => ({
          blocks: layout.blocks.length,
          paragraphs: layout.blocks.filter(
            (block) => block.kind === "paragraph",
          ).length,
          tables: layout.blocks.filter((block) => block.kind === "table")
            .length,
          headings: layout.blocks.flatMap((block, index) =>
            block.kind === "paragraph" && /^Heading[1-9]$/u.test(block.style)
              ? [
                  {
                    index,
                    style: block.style,
                    text: block.runs.map((run) => run.text).join(""),
                  },
                ]
              : [],
          ),
        }),
      );
    case "history": {
      const retained = schemas.history.parse(
        await application.query(operation, input, signal),
      );
      return {
        retained,
        model: { ...retained, entries: retained.entries.map(revisionSummary) },
      };
    }
    default: {
      const retained = await application.query(operation, input, signal);
      return { retained, model: retained };
    }
  }
}

// Text is the bounded model projection; structured content remains the exact
// original result. Recovery calls this same pure projection over the original receipt.
export function toolResult(reply: Reply, modelValue?: unknown) {
  const retained = "value" in reply ? { value: reply.value } : reply;
  let model: unknown =
    "value" in reply
      ? { value: modelValue === undefined ? reply.value : modelValue }
      : reply;
  if (size(model) > MODEL_LIMITS.textBytes)
    model = {
      projected: true,
      originalRetained: true,
      view: project(model),
      read: "Use read or assessment with the exact native identities and an explicit path to inspect retained evidence.",
    };
  const text = canonicalJSON(model);
  if (Buffer.byteLength(text) > MODEL_LIMITS.textBytes)
    throw new Error(
      "deliverables: model projection exceeded its byte capacity",
    );
  return {
    content: [{ type: "text" as const, text }],
    structuredContent: retained,
    ...("failure" in reply ? { isError: true } : {}),
  };
}

import { canonicalJSON, parseJSON, Refusal } from "@flame/backend-sdk/mcp";
import { isJSONNumber } from "@frame/sdk/json";
import { z } from "zod";
import {
  schemas,
  type Assessment,
  type Revision,
} from "../contracts/capability";
import { formatCatalogSchema, inspectionSchema } from "../contracts/content";
import { renderReportSchema, reviewReportSchema } from "../contracts/evidence";
import { LIMITS, MODEL_LIMITS } from "../contracts/limits";
import { projectNumbers } from "../contracts/json";
import type { Application, Operation } from "./application";
import type { Reply } from "./archive";

const selection = {
  path: z
    .array(z.string().max(256))
    .max(32)
    .optional()
    .describe(
      "Exact property names or decimal array indexes in the documented inspection view. Use [] to inspect its root. Omit for a summary.",
    ),
  offset: z
    .number()
    .int()
    .min(0)
    .max(LIMITS.inputBytes)
    .optional()
    .describe(
      "Continue the selected collection or string at its returned nextOffset.",
    ),
  limit: z
    .number()
    .int()
    .min(1)
    .max(MODEL_LIMITS.maximumTextCharacters)
    .optional()
    .describe(
      "At most 32 collection entries or 4096 Unicode code points. The byte budget can return fewer entries with nextOffset.",
    ),
};
export const modelInputs = {
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
};
type Selection = { path?: string[]; offset?: number; limit?: number };

export const modelDescriptions: Partial<Record<Operation, string>> = {
  catalog:
    "Read a short format/engine/limit overview first. Select format and section=create, edit or example for one exact authoring contract. section=engines or limits reads those facts. Large values support path/offset/limit; omitted entries remain retained, never inferred.",
  read: 'Read a native revision summary with its exact revision and sourceSha256. To inspect original content, supply both returned identities and path (for example ["inspection","items"] or ["revision","provenance"]). Paths address document, revision, inspection, processing and assessments. Document head and the append-ordered assessment list are current projections; native inspection and revision content are immutable. Append an entry\'s key to inspect it; continue at nextOffset with the same revision and hash. Text positions count Unicode code points. Retained inspection truncation is reported separately from projection paging.',
  assessment:
    'Read an exact render/review summary. Use path=["report","pages"], ["report","checks"], ["report","extractedText"], ["report","notes"] or ["assets"] for original evidence. Append an entry\'s key to inspect it and continue at nextOffset. The page tool returns a real PNG for visual review.',
};

function size(value: unknown): number {
  return Buffer.byteLength(canonicalJSON(value));
}
function record(value: unknown): value is Record<string, unknown> {
  return (
    value !== null &&
    typeof value === "object" &&
    !Array.isArray(value) &&
    !isJSONNumber(value)
  );
}
function kind(value: unknown): string {
  return value === null
    ? "null"
    : isJSONNumber(value)
      ? "number"
      : Array.isArray(value)
        ? "array"
        : typeof value;
}
function omitted(value: unknown) {
  return {
    omitted: true,
    type: kind(value),
    bytes: size(value),
    ...(Array.isArray(value)
      ? { length: value.length }
      : record(value)
        ? { keys: Object.keys(value).length }
        : typeof value === "string"
          ? { characters: Array.from(value).length }
          : {}),
  };
}

function selectedPath(selected: Selection): string[] {
  const path = selected.path ?? [];
  if (size(path) > MODEL_LIMITS.pathBytes)
    throw new Refusal(
      "projection_path_capacity",
      "inspection path exceeds its byte capacity",
    );
  if (
    selected.path === undefined &&
    (selected.offset !== undefined || selected.limit !== undefined)
  )
    throw new Refusal(
      "invalid_projection",
      "offset and limit require an explicit path",
    );
  return path;
}

function atPath(value: unknown, path: string[]): unknown {
  let source = value;
  for (const key of path) {
    if (Array.isArray(source)) {
      if (
        !/^(?:0|[1-9][0-9]*)$/u.test(key) ||
        !Number.isSafeInteger(Number(key)) ||
        Number(key) >= source.length
      )
        throw new Refusal(
          "inspection_path_not_found",
          "array index is outside the exact retained inspection",
        );
      source = source[Number(key)];
    } else if (record(source) && Object.hasOwn(source, key))
      source = source[key];
    else
      throw new Refusal(
        "inspection_path_not_found",
        "path is not present in the exact retained inspection",
      );
  }
  return source;
}

function textPage(
  source: string,
  path: string[],
  selected: Selection,
): Record<string, unknown> {
  const offset = selected.offset ?? 0;
  const characters = Array.from(source);
  if (offset > characters.length)
    throw new Refusal(
      "invalid_projection",
      "text offset exceeds the retained string",
    );
  let end = Math.min(
    characters.length,
    offset + (selected.limit ?? MODEL_LIMITS.textCharacters),
  );
  do {
    const result = {
      path,
      type: "string",
      unit: "unicode-code-points",
      offset,
      total: characters.length,
      text: characters.slice(offset, end).join(""),
      complete: offset === 0 && end === characters.length,
      ...(end < characters.length ? { nextOffset: end } : {}),
    };
    if (size(result) <= MODEL_LIMITS.projectionBytes) return result;
    end -= 1;
  } while (end > offset);
  throw new Refusal(
    "projection_capacity",
    "one text position exceeds the model projection capacity",
  );
}

function collectionPage(
  source: unknown[] | Record<string, unknown>,
  path: string[],
  selected: Selection,
): Record<string, unknown> {
  if (
    record(source) &&
    selected.offset === undefined &&
    selected.limit === undefined
  ) {
    const result = { path, type: "object", value: source, complete: true };
    if (size(result) <= MODEL_LIMITS.projectionBytes) return result;
  }
  const keys = Array.isArray(source)
    ? source.map((_, index) => String(index))
    : Object.keys(source).sort();
  const offset = selected.offset ?? 0;
  if (offset > keys.length)
    throw new Refusal(
      "invalid_projection",
      "collection offset exceeds the retained value",
    );
  const limit = selected.limit ?? MODEL_LIMITS.entries;
  if (limit > MODEL_LIMITS.maximumEntries)
    throw new Refusal(
      "invalid_projection",
      "collection pages admit at most 32 entries",
    );
  const entries: Array<Record<string, unknown>> = [];
  let next = offset;
  const result = () => ({
    path,
    type: kind(source),
    offset,
    total: keys.length,
    entries,
    complete:
      offset === 0 &&
      next === keys.length &&
      entries.every((entry) => !entry.omitted),
    ...(next < keys.length ? { nextOffset: next } : {}),
  });
  for (const key of keys.slice(offset, offset + limit)) {
    const value = Array.isArray(source) ? source[Number(key)] : source[key];
    const entry = {
      key,
      ...(size(value) <= MODEL_LIMITS.entryBytes ? { value } : omitted(value)),
    };
    entries.push(entry);
    next += 1;
    if (size(result()) > MODEL_LIMITS.projectionBytes) {
      entries.pop();
      next -= 1;
      if (!entries.length)
        throw new Refusal(
          "projection_capacity",
          "one retained key exceeds the model projection capacity",
        );
      break;
    }
  }
  return result();
}

export function project(
  value: unknown,
  selected: Selection = {},
): Record<string, unknown> {
  const path = selectedPath(selected);
  const source = atPath(value, path);
  if (typeof source === "string") return textPage(source, path, selected);
  if (Array.isArray(source) || record(source))
    return collectionPage(source, path, selected);
  if ((selected.offset ?? 0) !== 0 || selected.limit !== undefined)
    throw new Refusal("invalid_projection", "a scalar has no page offsets");
  return { path, type: kind(source), value: source, complete: true };
}

function revisionSummary(value: Revision) {
  return {
    id: value.id,
    documentId: value.documentId,
    ...(value.parent === undefined ? {} : { parent: value.parent }),
    createdAt: value.createdAt,
    operation: value.operation,
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
  if (["create", "edit", "import", "recalculate"].includes(operation))
    return revisionSummary(schemas.revision.parse(value));
  if (operation === "render" || operation === "review")
    return assessmentSummary(schemas.assessment.parse(value));
  return value;
}

type QuerySource = Pick<Application, "query">;
type QueryProjection = { retained: unknown; model: unknown };

function catalogProjection(
  application: QuerySource,
  input: Record<string, unknown>,
): QueryProjection {
  const request = modelInputs.catalog.parse(input);
  const retained = schemas.catalog.parse(application.query("catalog", {}));
  const formats = formatCatalogSchema.parse(
    JSON.parse(retained.formatsJSON),
  ).formats;
  const section = request.section ?? "overview";
  const format =
    request.format &&
    formats.find((format) => format.format === request.format);
  if (request.format && !format)
    throw new Refusal(
      "unsupported_format",
      "format is not in the original catalog",
    );
  let source: unknown;
  if (section === "overview") {
    source = {
      formats: (format ? [format] : formats).map((format) => ({
        format: format.format,
        canCreate: format.createSchemaJSON !== undefined,
        limitations: format.limitations,
      })),
      engines: JSON.parse(retained.enginesJSON),
      limits: JSON.parse(retained.limitsJSON),
    };
  } else if (section === "engines" || section === "limits") {
    if (format)
      throw new Refusal(
        "invalid_projection",
        "engine and limit views do not select a format",
      );
    source = parseJSON(
      section === "engines" ? retained.enginesJSON : retained.limitsJSON,
    );
  } else {
    if (!format)
      throw new Refusal(
        "invalid_projection",
        "select one format before reading its schema or example",
      );
    const original =
      section === "create"
        ? format.createSchemaJSON
        : section === "edit"
          ? format.editSchemaJSON
          : format.exampleJSON;
    if (original === undefined)
      throw new Refusal(
        "unsupported_format_operation",
        "the selected format does not define this operation",
      );
    source = parseJSON(original);
  }
  return {
    retained,
    model: {
      ...(request.format ? { format: request.format } : {}),
      section,
      view: project(source, request),
    },
  };
}

function documentProjection(
  application: QuerySource,
  input: Record<string, unknown>,
): QueryProjection {
  const request = modelInputs.read.parse(input);
  selectedPath(request);
  if (
    request.path !== undefined &&
    (request.revision === undefined || request.sourceSha256 === undefined)
  )
    throw new Refusal(
      "revision_required",
      "read a summary first, then pin its exact revision and sourceSha256 for inspection paging",
    );
  const retained = schemas.read.parse(
    application.query("read", { id: request.id, revision: request.revision }),
  );
  if (
    request.sourceSha256 !== undefined &&
    request.sourceSha256 !== retained.revision.sha256
  )
    throw new Refusal(
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

function assessmentProjection(
  application: QuerySource,
  input: Record<string, unknown>,
): QueryProjection {
  const request = modelInputs.assessment.parse(input);
  selectedPath(request);
  const retained = schemas.assessment.parse(
    application.query("assessment", {
      id: request.id,
      revision: request.revision,
      assessmentId: request.assessmentId,
    }),
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

export function queryProjection(
  application: QuerySource,
  operation: Operation,
  input: Record<string, unknown>,
): QueryProjection {
  switch (operation) {
    case "catalog":
      return catalogProjection(application, input);
    case "read":
      return documentProjection(application, input);
    case "assessment":
      return assessmentProjection(application, input);
    case "history": {
      const retained = schemas.history.parse(
        application.query(operation, input),
      );
      return {
        retained,
        model: { ...retained, entries: retained.entries.map(revisionSummary) },
      };
    }
    default: {
      const retained = application.query(operation, input);
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

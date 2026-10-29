import { z } from "zod";
import { PROTOCOL, type CapabilityDescriptor } from "@qimu/node";
import { LIMITS } from "./limits";
import { formatSchema } from "./content";

import { METHODS } from "./methods";
export { METHODS } from "./methods";

const id = z.string().regex(/^[a-f0-9]{64}$/);
const format = formatSchema;
const title = z.string().min(1).max(180);
const timestamp = z.string().min(1).max(40);
const empty = z.strictObject({});
const json = z.string().min(2).max(LIMITS.inputBytes);
const provenance = z.strictObject({
  sources: z
    .array(
      z.strictObject({
        title: z.string().min(1).max(300),
        uri: z.string().max(2048),
        note: z.string().max(2000),
      }),
    )
    .max(LIMITS.provenanceSources),
  assumptions: z.array(z.string().min(1).max(2000)).max(64),
  units: z.string().max(2000),
  reproduction: z.string().max(12000),
});
const asset = z.strictObject({
  id,
  filename: z.string().min(1).max(240),
  mime: z.string().min(1).max(100),
  size: z.number().int().min(1).max(LIMITS.renderBytes),
  sha256: id,
});
const revision = z.strictObject({
  id,
  documentId: id,
  parent: id.optional(),
  createdAt: timestamp,
  operation: z.enum(["create", "import", "edit", "recalculate"]),
  format,
  sha256: id,
  size: z.number().int().min(1).max(LIMITS.fileBytes),
  provenance,
  native: asset,
});
const header = z.strictObject({
  id,
  title,
  format,
  head: id,
  createdAt: timestamp,
});
const assessment = z.strictObject({
  id,
  revision: id,
  sourceSha256: id,
  createdAt: timestamp,
  kind: z.enum(["render", "review"]),
  reportJSON: z.string().max(LIMITS.reportBytes),
  assets: z.array(asset).max(LIMITS.renderPages + 2),
});
const page = z.strictObject({
  before: id.optional(),
  limit: z.number().int().min(1).max(LIMITS.pageSize),
});
const selected = z.strictObject({ id, revision: id });
const expected = z.strictObject({ id, expectedRevision: id });
const uploaded = z.strictObject({
  id,
  sha256: id,
  size: z.number().int().min(1).max(LIMITS.fileBytes),
  received: z.number().int().min(0).max(LIMITS.fileBytes),
});

export const schemas = Object.freeze({
  empty,
  format,
  provenance,
  asset,
  revision,
  header,
  assessment,
  selected,
  expected,
  page,
  uploaded,
  catalog: z.strictObject({
    formatsJSON: json,
    enginesJSON: json,
    limitsJSON: json,
  }),
  list: z.strictObject({
    entries: z.array(header).max(LIMITS.pageSize),
    next: id.optional(),
  }),
  readInput: z.strictObject({ id, revision: id.optional() }),
  read: z.strictObject({
    document: header,
    revision,
    inspectionJSON: z.string().max(LIMITS.reportBytes),
    processingJSON: z.string().max(LIMITS.reportBytes).optional(),
    assessments: z
      .array(assessment.omit({ reportJSON: true }))
      .max(LIMITS.assessments),
  }),
  assessmentInput: selected.extend({ assessmentId: id }),
  historyInput: page.extend({ id }),
  history: z.strictObject({
    entries: z.array(revision).max(LIMITS.pageSize),
    next: id.optional(),
  }),
  create: z.strictObject({
    title,
    format: format.exclude(["pdf"]),
    contentJSON: json,
    provenance,
  }),
  edit: expected.extend({ operationsJSON: json }),
  upload: z.strictObject({
    size: z.number().int().min(1).max(LIMITS.fileBytes),
    sha256: id,
  }),
  append: z.strictObject({
    uploadId: id,
    offset: z.number().int().min(0).max(LIMITS.fileBytes),
    base64: z
      .string()
      .min(4)
      .max(Math.ceil(LIMITS.chunkBytes / 3) * 4),
  }),
  uploadId: z.strictObject({ uploadId: id }),
  import: z.strictObject({ title, format, uploadId: id, provenance }),
  review: selected.extend({
    renderId: id,
    pages: z
      .array(z.number().int().min(1).max(LIMITS.renderPages))
      .min(1)
      .max(LIMITS.renderPages),
    verdict: z.enum(["accepted", "changes_requested"]),
    notes: z.string().min(1).max(12000),
  }),
  export: selected.extend({
    kind: z.enum(["native", "pdf", "bundle", "csv"]),
    sheetPart: z.string().min(1).max(240).optional(),
  }),
  download: z.strictObject({
    assetId: id,
    offset: z.number().int().min(0).max(LIMITS.renderBytes),
    length: z.number().int().min(1).max(LIMITS.chunkBytes),
  }),
  chunk: z.strictObject({
    asset: asset,
    offset: z.number().int().min(0).max(LIMITS.renderBytes),
    base64: z.string().max(Math.ceil(LIMITS.chunkBytes / 3) * 4),
    next: z.number().int().min(0).max(LIMITS.renderBytes),
    done: z.boolean(),
  }),
});

export type Format = z.infer<typeof format>;
export type Provenance = z.infer<typeof provenance>;
export type Asset = z.infer<typeof asset>;
export type Revision = z.infer<typeof revision>;
export type Header = z.infer<typeof header>;
export type Assessment = z.infer<typeof assessment>;
export type DocumentRead = z.infer<typeof schemas.read>;

function method(
  operation: keyof typeof METHODS,
  kind: "query" | "command",
  description: string,
  input: z.ZodType,
  output: z.ZodType,
) {
  return { operation, kind, description, input, output };
}

export const definitions = [
  method(
    "catalog",
    "query",
    "Read the exact supported native creation/edit schemas, format limits and configured rendering engines before creating or editing deliverables.",
    empty,
    schemas.catalog,
  ),
  method(
    "list",
    "query",
    "List retained deliverables newest first. Content and evidence remain owned by exact immutable native revisions.",
    page,
    schemas.list,
  ),
  method(
    "read",
    "query",
    "Inspect a deliverable and exact native revision, its editable item identities and source-bound checks. Omitting revision reads the current head once.",
    schemas.readInput,
    schemas.read,
  ),
  method(
    "assessment",
    "query",
    "Read one exact render or review report. List headers are projections and do not carry another mutable assessment.",
    schemas.assessmentInput,
    assessment,
  ),
  method(
    "history",
    "query",
    "Read immutable revision history; previous previews and reviews do not apply to later revisions.",
    schemas.historyInput,
    schemas.history,
  ),
  method(
    "create",
    "command",
    "Create an editable DOCX, XLSX or PPTX from the catalog's content schema. Author provenance is not independently verified evidence. Formula results require explicit recalculation.",
    schemas.create,
    revision,
  ),
  method(
    "edit",
    "command",
    "Apply catalog-defined local native edits to the exact current revision. Unaffected package parts are preserved; stale revisions are refused. A new revision has no inherited render or review.",
    schemas.edit,
    revision,
  ),
  method(
    "upload",
    "command",
    "Reserve a bounded native-file upload for the exact size and SHA-256. No host path or URL is read.",
    schemas.upload,
    uploaded,
  ),
  method(
    "uploads",
    "query",
    "Read admitted staging uploads and their current acknowledged offsets. Reselect only bytes matching the declared SHA-256 to resume.",
    empty,
    z.array(uploaded).max(LIMITS.uploads),
  ),
  method(
    "append",
    "command",
    "Append one canonical base64 chunk at the exact acknowledged upload offset. Reuse the original command receipt after transport loss, never invent a retry identity.",
    schemas.append,
    uploaded,
  ),
  method(
    "discard",
    "command",
    "Discard the specified staging upload. This does not remove a published deliverable.",
    schemas.uploadId,
    z.boolean(),
  ),
  method(
    "import",
    "command",
    "Import the completed, hash-verified native upload, preserving its admitted original bytes. Unsafe or unsupported packages are refused without silent feature stripping.",
    schemas.import,
    revision,
  ),
  method(
    "render",
    "command",
    "Render the exact native revision through configured confined LibreOffice/Poppler. Record actual engine versions, PDF/page hashes and extraction checks; successful rendering does not establish visual correctness.",
    selected,
    assessment,
  ),
  method(
    "recalculate",
    "command",
    "Recalculate an XLSX through confined LibreOffice into a new native revision. Conversion may change Office formatting; the exact original is retained. Inspect resulting formula errors and missing caches.",
    expected,
    revision,
  ),
  method(
    "review",
    "command",
    "Record the caller's visual review of enumerated pages from an exact render. This is attributed review evidence, not an automatic quality certification or approval to publish.",
    schemas.review,
    assessment,
  ),
  method(
    "export",
    "command",
    "Prepare the exact native revision, its existing rendered PDF, one XLSX sheet as CSV (sheetPart required), or a delivery ZIP with native source, current derivatives and evidence manifest. CSV refuses missing/error formula caches and protects formula-like strings. This never sends or publishes a file.",
    schemas.export,
    asset,
  ),
  method(
    "download",
    "query",
    "Read bounded bytes from an exact immutable asset. Verify the complete size and SHA-256 before saving; file saving is a separate host operation.",
    schemas.download,
    schemas.chunk,
  ),
  method(
    "remove",
    "command",
    "Remove one deliverable and its native/derived bytes only if the expected current revision still matches. Original receipts remain and cannot resurrect removed content.",
    expected,
    z.boolean(),
  ),
];

const descriptor: CapabilityDescriptor = {
  protocol: PROTOCOL,
  reference: "flame.deliverables.v1",
  composition: "collection",
  scope: "installation",
  methods: definitions.map(
    ({ operation, kind, description, input, output }) => ({
      name: METHODS[operation],
      kind,
      target: "installation",
      description,
      input: z.toJSONSchema(input),
      output: z.toJSONSchema(output),
    }),
  ),
};
export default descriptor;

import { z } from "zod";
import { PROTOCOL, type CapabilityDescriptor } from "@flame/backend-sdk/mcp";
import { FIELD_LIMITS, LIMITS } from "./limits";
import { STAGED_OPERATIONS, formatSchema, officeFormatSchema } from "./content";
import { authorSchema } from "./evidence";

import { METHODS } from "./methods";
export { METHODS } from "./methods";

const stagedEdits = Object.entries(STAGED_OPERATIONS)
  .map(([format, operations]) => `${format.toUpperCase()} ${operations.join(", ")}`)
  .join(". ");

const id = z.string().regex(/^[a-f0-9]{64}$/);
// Runtime owns Project identity; a deliverable only records the one it was made in.
const project = z.string().min(1).max(256);
const title = z.string().min(1).max(FIELD_LIMITS.title);
const timestamp = z.string().min(1).max(40);
const empty = z.strictObject({});
const json = z.string().min(2).max(LIMITS.inputBytes);
const provenance = z.strictObject({
  sources: z
    .array(
      z.strictObject({
        title: z.string().min(1).max(FIELD_LIMITS.sourceTitle),
        uri: z.string().max(FIELD_LIMITS.uri),
        note: z.string().max(FIELD_LIMITS.note),
      }),
    )
    .max(LIMITS.provenanceSources),
  assumptions: z.array(z.string().min(1).max(FIELD_LIMITS.note)).max(64),
  units: z.string().max(FIELD_LIMITS.units),
  reproduction: z.string().max(FIELD_LIMITS.reproduction),
});
const asset = z.strictObject({
  id,
  filename: z.string().min(1).max(240),
  mime: z.string().min(1).max(100),
  size: z.number().int().min(1).max(LIMITS.renderBytes),
  sha256: id,
});
const revisionFacts = {
  id,
  documentId: id,
  createdAt: timestamp,
  format: formatSchema,
  sha256: id,
  size: z.number().int().min(1).max(LIMITS.fileBytes),
  provenance,
  native: asset,
};
// A restoration names the earlier revision whose native bytes it carries.
const revision = z.discriminatedUnion("operation", [
  z.strictObject({
    ...revisionFacts,
    parent: id.optional(),
    operation: z.enum(["create", "import", "copy", "edit", "recalculate"]),
  }),
  z.strictObject({
    ...revisionFacts,
    parent: id,
    operation: z.literal("restore"),
    restored: id,
  }),
]);
const header = z.strictObject({
  id,
  title,
  format: formatSchema,
  head: id,
  createdAt: timestamp,
  project: project.optional(),
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
// A comment is anchored where it was made: the revision, the editor's object
// identity (a slide or element, "sheet!A1" or a block) and the object's text
// then, so the comment stays readable after the object changes or goes.
const comment = z.string().trim().min(1).max(FIELD_LIMITS.comment);
const anchor = z.strictObject({
  revision: id,
  object: z.string().min(1).max(2048).optional(),
  quote: z.string().max(FIELD_LIMITS.quote).optional(),
});
const threadMessage = z.strictObject({
  id,
  author: authorSchema,
  body: comment,
  createdAt: timestamp,
  // A revision the message points to, such as the one that addresses it.
  revision: id.optional(),
});
const threadStatus = z.enum(["open", "resolved"]);
export const THREAD_STATUSES = threadStatus.options;
const thread = z.strictObject({
  id,
  documentId: id,
  anchor,
  status: threadStatus,
  createdAt: timestamp,
  updatedAt: timestamp,
  messages: z.array(threadMessage).min(1).max(LIMITS.threadMessages),
});
const threadTarget = z.strictObject({ id, thread: id });
// Edits proposed against the exact revision their author read. Adopting them
// into a draft and saving is the person's decision; settling records it.
const suggestionFields = z.strictObject({
  id,
  documentId: id,
  revision: id,
  note: z.string().max(FIELD_LIMITS.comment),
  operationsJSON: json,
  author: authorSchema,
  createdAt: timestamp,
});
const pendingStatus = z.literal("pending");
const suggestionOutcome = z.enum(["adopted", "dismissed"]);
export const SUGGESTION_STATUSES = [
  pendingStatus.value,
  ...suggestionOutcome.options,
] as const;
const pendingSuggestion = suggestionFields.extend({ status: pendingStatus });
const settledSuggestion = suggestionFields.extend({
  status: suggestionOutcome,
  settledAt: timestamp,
});
const suggestion = z.discriminatedUnion("status", [
  pendingSuggestion,
  settledSuggestion,
]);
// A layout derived from one native revision on each read.
const projection = z.strictObject({
  document: header,
  revision: id,
  sourceSha256: id,
  layoutJSON: z.string().max(LIMITS.reportBytes),
});
const page = z.strictObject({
  before: id.optional(),
  limit: z.number().int().min(1).max(LIMITS.pageSize),
});
const listInput = page.extend({ project: project.optional() });
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
  format: formatSchema,
  provenance,
  asset,
  revision,
  header,
  assessment,
  selected,
  expected,
  page,
  listInput,
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
  slides: projection,
  sheets: projection,
  blocks: projection,
  assessmentInput: selected.extend({ assessmentId: id }),
  historyInput: page.extend({ id }),
  history: z.strictObject({
    entries: z.array(revision).max(LIMITS.pageSize),
    next: id.optional(),
  }),
  create: z.strictObject({
    title,
    format: officeFormatSchema,
    contentJSON: json,
    provenance,
    project: project.optional(),
  }),
  edit: expected.extend({ operationsJSON: json }),
  restore: expected.extend({ revision: id }),
  copy: selected.extend({ title, project: project.optional() }),
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
  import: z.strictObject({
    title,
    format: formatSchema,
    uploadId: id,
    provenance,
    project: project.optional(),
  }),
  thread,
  threadMessage,
  threadTarget,
  threadsInput: page.extend({
    id,
    status: threadStatus.optional(),
  }),
  threads: z.strictObject({
    entries: z.array(thread).max(LIMITS.pageSize),
    next: id.optional(),
  }),
  comment: anchor.extend({ id, body: comment }),
  suggestion,
  settledSuggestion,
  suggestionsInput: page.extend({
    id,
    status: z.enum(SUGGESTION_STATUSES).optional(),
  }),
  suggestions: z.strictObject({
    entries: z.array(suggestion).max(LIMITS.pageSize),
    next: id.optional(),
  }),
  suggest: z.strictObject({
    id,
    revision: id,
    note: z.string().max(FIELD_LIMITS.comment),
    operationsJSON: json,
  }),
  settle: z.strictObject({
    id,
    suggestion: id,
    outcome: suggestionOutcome,
  }),
  reply: threadTarget.extend({ body: comment, revision: id.optional() }),
  review: selected.extend({
    renderId: id,
    pages: z
      .array(z.number().int().min(1).max(LIMITS.renderPages))
      .min(1)
      .max(LIMITS.renderPages),
    verdict: z.enum(["accepted", "changes_requested"]),
    notes: z.string().min(1).max(FIELD_LIMITS.reviewNotes),
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

export type Provenance = z.infer<typeof provenance>;
export type Asset = z.infer<typeof asset>;
export type Revision = z.infer<typeof revision>;
export type Header = z.infer<typeof header>;
export type Assessment = z.infer<typeof assessment>;
export type DocumentRead = z.infer<typeof schemas.read>;
export type ThreadValue = z.infer<typeof thread>;
export type ThreadMessage = z.infer<typeof threadMessage>;
export type SuggestionValue = z.infer<typeof suggestion>;
export type PendingSuggestion = z.infer<typeof pendingSuggestion>;
export type SettledSuggestion = z.infer<typeof settledSuggestion>;

function method(
  operation: keyof typeof METHODS,
  kind: "query" | "command",
  description: string,
  input: z.ZodType,
  output: z.ZodType,
  audience: "model" | "host",
) {
  return { operation, kind, description, input, output, audience };
}

export const definitions = [
  method(
    "catalog",
    "query",
    "Read the exact supported native creation/edit schemas, format limits and configured rendering engines before creating or editing deliverables.",
    empty,
    schemas.catalog,
    "model",
  ),
  method(
    "list",
    "query",
    "List retained deliverables newest first, only the caller's Project's own when it works in a Project. Content and evidence remain owned by exact immutable native revisions.",
    listInput,
    schemas.list,
    "model",
  ),
  method(
    "read",
    "query",
    "Inspect a deliverable and exact native revision, its editable item identities and source-bound checks. Omitting revision reads the current head once.",
    schemas.readInput,
    schemas.read,
    "model",
  ),
  method(
    "slides",
    "query",
    "Project a PPTX revision's slides, element identities, frames, text runs, notes, chart data and the slide layouts insertSlide accepts from its native bytes. Omitting revision reads the current head once. Use these identities with expect values in catalog-defined presentation edits.",
    schemas.readInput,
    schemas.slides,
    "model",
  ),
  method(
    "sheets",
    "query",
    "Project an XLSX revision's sheets and cells from its native bytes: each cell's exact setCell expect, shown value and reproducing setCell value, plus locked ranges setCell refuses. Omitting revision reads the current head once.",
    schemas.readInput,
    schemas.sheets,
    "model",
  ),
  method(
    "blocks",
    "query",
    "Project a DOCX revision's body blocks from its native bytes: paragraphs with style, list and alignment facts and their text runs, tables and other blocks, each with the block id and exact-XML expect that block edits name. Omitting revision reads the current head once.",
    schemas.readInput,
    schemas.blocks,
    "model",
  ),
  method(
    "assessment",
    "query",
    "Read one exact render or review report. List headers are projections and do not carry another mutable assessment.",
    schemas.assessmentInput,
    assessment,
    "model",
  ),
  method(
    "history",
    "query",
    "Read immutable revision history; previous previews and reviews do not apply to later revisions.",
    schemas.historyInput,
    schemas.history,
    "model",
  ),
  method(
    "create",
    "command",
    "Create an editable DOCX, XLSX or PPTX from the catalog's content schema. Author provenance is not independently verified evidence. Formula results require explicit recalculation.",
    schemas.create,
    revision,
    "model",
  ),
  method(
    "edit",
    "command",
    "Apply catalog-defined local native edits to the exact current revision. Unaffected package parts are preserved; stale revisions are refused. A new revision has no inherited render or review.",
    schemas.edit,
    revision,
    "model",
  ),
  method(
    "upload",
    "command",
    "Reserve a bounded native-file upload for the exact size and SHA-256. No host path or URL is read.",
    schemas.upload,
    uploaded,
    "host",
  ),
  method(
    "uploads",
    "query",
    "Read admitted staging uploads and their current acknowledged offsets. Reselect only bytes matching the declared SHA-256 to resume.",
    empty,
    z.array(uploaded).max(LIMITS.uploads),
    "model",
  ),
  method(
    "append",
    "command",
    "Append one canonical base64 chunk at the exact acknowledged upload offset. Reuse the original command receipt after transport loss, never invent a retry identity.",
    schemas.append,
    uploaded,
    "host",
  ),
  method(
    "discard",
    "command",
    "Discard the specified staging upload. This does not remove a published deliverable.",
    schemas.uploadId,
    z.boolean(),
    "host",
  ),
  method(
    "import",
    "command",
    "Import the completed, hash-verified native upload, preserving its admitted original bytes. Unsafe or unsupported packages are refused without silent feature stripping.",
    schemas.import,
    revision,
    "model",
  ),
  method(
    "render",
    "command",
    "Render the exact native revision through configured confined LibreOffice/Poppler. Record actual engine versions, PDF/page hashes and extraction checks; successful rendering does not establish visual correctness.",
    selected,
    assessment,
    "model",
  ),
  method(
    "recalculate",
    "command",
    "Recalculate an XLSX through confined LibreOffice into a new native revision. Conversion may change Office formatting; the exact original is retained. Inspect resulting formula errors and missing caches.",
    expected,
    revision,
    "model",
  ),
  method(
    "restore",
    "command",
    "Make an earlier revision of the same deliverable current again as a new revision with its exact native bytes. The exact current revision is required; history is kept, and the new revision has no inherited render or review.",
    schemas.restore,
    revision,
    "model",
  ),
  method(
    "copy",
    "command",
    "Start a new deliverable from an exact revision of another, such as a template. Its first revision carries that revision's native bytes, including masters, layouts, theme, styles and sample content, and records the source revision and its hash as processing evidence; later changes to the source do not affect it.",
    schemas.copy,
    revision,
    "model",
  ),
  method(
    "review",
    "command",
    "Record the caller's visual review of enumerated pages from an exact render. This is attributed review evidence, not an automatic quality certification or approval to publish.",
    schemas.review,
    assessment,
    "model",
  ),
  method(
    "export",
    "command",
    "Prepare the exact native revision, its existing rendered PDF, one XLSX sheet as CSV (sheetPart required), or a delivery ZIP with native source, current derivatives and evidence manifest. CSV refuses missing/error formula caches and protects formula-like strings. This never sends or publishes a file.",
    schemas.export,
    asset,
    "model",
  ),
  method(
    "download",
    "query",
    "Read bounded bytes from an exact immutable asset. Verify the complete size and SHA-256 before saving; file saving is a separate host operation.",
    schemas.download,
    schemas.chunk,
    "host",
  ),
  method(
    "threads",
    "query",
    "List a deliverable's comment threads, newest first: each anchor (the revision, object identity and quoted text it was made on), status and messages. A thread whose object no longer exists keeps its anchor and quote.",
    schemas.threadsInput,
    schemas.threads,
    "model",
  ),
  method(
    "comment",
    "command",
    "Open a comment thread anchored to an exact revision and optionally an object in it, with the object's text then.",
    schemas.comment,
    thread,
    "host",
  ),
  method(
    "reply",
    "command",
    "Reply to a comment thread, optionally naming the revision that addresses it. Replying never resolves a thread; the person who asked decides.",
    schemas.reply,
    thread,
    "model",
  ),
  method(
    "resolve",
    "command",
    "Resolve an open comment thread.",
    threadTarget,
    thread,
    "host",
  ),
  method(
    "reopen",
    "command",
    "Reopen a resolved comment thread.",
    threadTarget,
    thread,
    "host",
  ),
  method(
    "suggestions",
    "query",
    "List a deliverable's suggested edits newest first, with the revision each was made against and whether the person adopted or dismissed it.",
    schemas.suggestionsInput,
    schemas.suggestions,
    "model",
  ),
  method(
    "suggest",
    "command",
    `Propose edits the person can review in the live editor for an exact revision. Nothing changes until they adopt the edits into their draft and save it. The supported operations are ${stagedEdits}. For replaceText, use a text run from the revision's slides or blocks projection. Use edit for direct changes, including text outside those projections.`,
    schemas.suggest,
    pendingSuggestion,
    "model",
  ),
  method(
    "settle",
    "command",
    "Record that the person adopted a pending suggestion into their draft or dismissed it.",
    schemas.settle,
    settledSuggestion,
    "host",
  ),
  method(
    "remove",
    "command",
    "Remove one deliverable and its native/derived bytes only if the expected current revision still matches. Original receipts remain and cannot resurrect removed content.",
    expected,
    z.boolean(),
    "model",
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

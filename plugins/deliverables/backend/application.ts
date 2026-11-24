import {
  Refusal,
  canonicalJSON,
  type CallContext,
} from "@flame/backend-sdk/mcp";
import { toolCommandId } from "@flame/backend-sdk";
import { z } from "zod";
import {
  schemas,
  METHODS,
  type Asset,
  type Assessment,
  type DocumentRead,
  type Provenance,
} from "../contracts/capability";
import { LIMITS } from "../contracts/limits";
import { projectNumbers, type FrozenJSONValue } from "@frame/sdk/json";
import {
  engineCatalogSchema,
  renderReportSchema,
  reviewReportSchema,
} from "../contracts/evidence";
import { Archive } from "./archive";
import { ASSET_MIME, Document, digest, filename, revision } from "./document";
import { Engine } from "./engine";
import {
  catalog,
  encodeArchive,
  hasPNGSignature,
  type Format,
} from "./formats";
import { Formats } from "./format-process";

type Input = Record<string, unknown>;
export type Operation = keyof typeof METHODS;

export function decodeInput(schema: z.ZodType, value: unknown): unknown {
  // Envelope numbers are bounded integer counts and offsets. Content numbers
  // stay inside the format owner's original JSON string until format admission.
  // Frame admits method and tool input as frozen JSON.
  return schema.parse(projectNumbers(value as FrozenJSONValue));
}

interface InitialSource {
  title: string;
  format: Format;
  project?: string;
  provenance: Provenance;
  operation: "create" | "import";
  bytes: Uint8Array;
}

interface RevisionSource {
  id: string;
  sha256: string;
  format: Format;
  bytes: Uint8Array;
}

interface Derived {
  bytes: Uint8Array;
  processing?: unknown;
}

function retainedJSON(value: unknown): string {
  const text = canonicalJSON(value);
  if (Buffer.byteLength(text) > LIMITS.reportBytes)
    throw new Refusal(
      "report_capacity",
      "inspection exceeds the retained evidence capacity",
    );
  return text;
}

function base64(value: string): Uint8Array {
  const result = Buffer.from(value, "base64");
  if (!result.byteLength || result.toString("base64") !== value)
    throw new Refusal(
      "invalid_chunk",
      "upload chunk requires nonempty canonical base64",
    );
  return result;
}

function asset(
  command: string,
  name: string,
  mime: string,
  bytes: Uint8Array,
): Asset {
  return schemas.asset.parse({
    id: digest(canonicalJSON({ command, name, sha256: digest(bytes) })),
    filename: name,
    mime,
    size: bytes.byteLength,
    sha256: digest(bytes),
  });
}

export class Application {
  readonly #archive: Archive;
  readonly #engine: Engine;
  readonly #formats: Formats;

  constructor(archive: Archive, engine: Engine, formats: Formats) {
    this.#archive = archive;
    this.#engine = engine;
    this.#formats = formats;
  }

  catalog() {
    return schemas.catalog.parse({
      formatsJSON: canonicalJSON(catalog()),
      enginesJSON: canonicalJSON(
        engineCatalogSchema.parse(this.#engine.catalog()),
      ),
      limitsJSON: canonicalJSON(LIMITS),
    });
  }

  read(input: z.infer<typeof schemas.readInput>): DocumentRead {
    const header = this.#archive.document(input.id).snapshot();
    const source = this.#archive.revision(
      input.id,
      input.revision ?? header.head,
    );
    return schemas.read.parse({
      document: header,
      revision: source.metadata,
      inspectionJSON: retainedJSON(source.inspection),
      ...(source.processing === undefined
        ? {}
        : { processingJSON: retainedJSON(source.processing) }),
      assessments: this.#archive
        .assessments(input.id, source.metadata.id)
        .map(({ reportJSON: _report, ...header }) => header),
    });
  }

  async query(
    operation: Operation,
    input: Input,
    signal: AbortSignal,
  ): Promise<unknown> {
    switch (operation) {
      case "catalog":
        return this.catalog();
      case "list": {
        const value = schemas.listInput.parse(input);
        return this.#archive.list(value.before, value.limit, value.project);
      }
      case "read":
        return this.read(schemas.readInput.parse(input));
      case "assessment": {
        const value = schemas.assessmentInput.parse(input);
        this.#archive.revision(value.id, value.revision);
        const result = this.#archive
          .assessments(value.id, value.revision)
          .find((entry) => entry.id === value.assessmentId);
        if (!result)
          throw new Refusal(
            "assessment_not_found",
            "assessment was not found for the exact native revision",
          );
        return result;
      }
      case "history": {
        const value = schemas.historyInput.parse(input);
        return this.#archive.history(value.id, value.before, value.limit);
      }
      case "slides": {
        const value = schemas.readInput.parse(input);
        const header = this.#archive.document(value.id).snapshot();
        const source = this.#archive.revision(
          value.id,
          value.revision ?? header.head,
        );
        if (source.metadata.format !== "pptx")
          throw new Refusal(
            "invalid_format",
            "slide projection requires a native PPTX revision",
          );
        const layoutJSON = canonicalJSON(
          await this.#formats.slides(source.bytes, signal),
        );
        if (Buffer.byteLength(layoutJSON) > LIMITS.reportBytes)
          throw new Refusal(
            "report_capacity",
            "slide projection exceeds its report capacity",
          );
        return schemas.slides.parse({
          document: header,
          revision: source.metadata.id,
          sourceSha256: source.metadata.sha256,
          layoutJSON,
        });
      }
      case "uploads":
        return this.#archive.uploads();
      case "download": {
        const value = schemas.download.parse(input);
        const file = this.#archive.chunk(
          value.assetId,
          value.offset,
          value.length,
        );
        const chunk = file.bytes;
        const next = value.offset + chunk.byteLength;
        return schemas.chunk.parse({
          asset: file.metadata,
          offset: value.offset,
          base64: Buffer.from(chunk).toString("base64"),
          next,
          done: next === file.metadata.size,
        });
      }
      default:
        throw new Error("deliverables: operation is not a query");
    }
  }

  page(assetId: string) {
    const file = this.#archive.asset(assetId);
    if (
      file.metadata.mime !== ASSET_MIME.png ||
      file.metadata.size > LIMITS.previewFileBytes ||
      !hasPNGSignature(file.bytes)
    )
      throw new Refusal(
        "invalid_page",
        "preview requires a retained PNG page of at most 4 MiB",
      );
    return {
      metadata: file.metadata,
      base64: Buffer.from(file.bytes).toString("base64"),
    };
  }

  async command(
    context: CallContext,
    operation: Operation,
    input: Input,
    kind: "command" | "tool" = "command",
  ): Promise<unknown> {
    if (!context.invocation)
      throw new Error("deliverables: trusted invocation is required");
    const id = kind === "tool" ? toolCommandId(context) : context.commandId;
    if (!id) throw new Error("deliverables: original command is required");
    return this.#archive.command(
      {
        id,
        kind,
        method: kind === "tool" ? operation : METHODS[operation],
        input: context.requestInput,
        inputDigest: context.inputDigest,
        invocation: context.invocation,
      },
      context.signal,
      (command) => this.#prepare(context, operation, input, command),
      operation === "remove" || operation === "discard"
        ? "reclaim"
        : "ordinary",
    );
  }

  async #prepare(
    context: CallContext,
    operation: Operation,
    input: Input,
    command: string,
  ): Promise<() => unknown> {
    const now = new Date().toISOString();
    switch (operation) {
      case "create":
        return this.#prepareCreate(
          context,
          schemas.create.parse(input),
          command,
          now,
        );
      case "upload": {
        const value = schemas.upload.parse(input);
        return () =>
          this.#archive.beginUpload(
            digest(`upload:${command}`),
            value.size,
            value.sha256,
          );
      }
      case "append": {
        const value = schemas.append.parse(input);
        const bytes = base64(value.base64);
        return () =>
          this.#archive.appendUpload(value.uploadId, value.offset, bytes);
      }
      case "discard": {
        const value = schemas.uploadId.parse(input);
        return () => this.#archive.discardUpload(value.uploadId);
      }
      case "import":
        return this.#prepareImport(
          context,
          schemas.import.parse(input),
          command,
          now,
        );
      case "edit": {
        const value = schemas.edit.parse(input);
        return this.#prepareRevision(context, value, command, now, (source) =>
          this.#formats
            .edit(
              source.format,
              source.bytes,
              value.operationsJSON,
              context.signal,
            )
            .then((bytes) => ({ bytes })),
        );
      }
      case "recalculate":
        return this.#prepareRevision(
          context,
          schemas.expected.parse(input),
          command,
          now,
          (source) => this.#recalculate(source, context.signal),
          "recalculate",
        );
      case "render":
        return this.#prepareRender(
          context,
          schemas.selected.parse(input),
          command,
          now,
        );
      case "review":
        return this.#prepareReview(
          context,
          schemas.review.parse(input),
          command,
          now,
        );
      case "export":
        return this.#prepareExport(
          context,
          schemas.export.parse(input),
          command,
          now,
        );
      case "remove": {
        const value = schemas.expected.parse(input);
        return () => this.#archive.remove(value.id, value.expectedRevision);
      }
      default:
        throw new Error("deliverables: operation is not a command");
    }
  }

  // Inspection is published as retained evidence, so content whose inspection
  // cannot be retained is refused before any state changes.
  async #inspect(format: Format, bytes: Uint8Array, signal: AbortSignal) {
    const inspection = await this.#formats.inspect(format, bytes, signal);
    retainedJSON(inspection);
    return inspection;
  }

  async #initial(
    context: CallContext,
    command: string,
    now: string,
    source: InitialSource,
  ) {
    const inspection = await this.#inspect(
      source.format,
      source.bytes,
      context.signal,
    );
    const initial = revision({
      command,
      documentId: digest(`document:${command}`),
      title: source.title,
      format: source.format,
      operation: source.operation,
      createdAt: now,
      bytes: source.bytes,
      provenance: source.provenance,
    });
    const document = Document.create(source.title, initial, source.project);
    return () =>
      this.#archive.publish(document, initial, source.bytes, inspection);
  }

  async #prepareCreate(
    context: CallContext,
    value: z.infer<typeof schemas.create>,
    command: string,
    now: string,
  ) {
    const bytes = await this.#formats.create(
      value.format,
      value.contentJSON,
      context.signal,
    );
    return this.#initial(context, command, now, {
      ...value,
      operation: "create",
      bytes,
    });
  }

  async #prepareImport(
    context: CallContext,
    value: z.infer<typeof schemas.import>,
    command: string,
    now: string,
  ) {
    const staged = this.#archive.upload(value.uploadId);
    if (
      staged.metadata.received !== staged.metadata.size ||
      digest(staged.bytes) !== staged.metadata.sha256
    )
      throw new Refusal(
        "incomplete_upload",
        "native upload is incomplete or differs from its declared SHA-256",
      );
    const publish = await this.#initial(context, command, now, {
      ...value,
      operation: "import",
      bytes: staged.bytes,
    });
    return () => {
      this.#archive.discardUpload(value.uploadId);
      return publish();
    };
  }

  async #recalculate(
    source: RevisionSource,
    signal: AbortSignal,
  ): Promise<Derived> {
    if (source.format !== "xlsx")
      throw new Refusal(
        "invalid_format",
        "recalculation requires a native XLSX revision",
      );
    const result = await this.#engine.recalculate(source.bytes, signal);
    return {
      bytes: result.bytes,
      processing: {
        sourceRevision: source.id,
        sourceSha256: source.sha256,
        outputSha256: digest(result.bytes),
        observation: result.report,
      },
    };
  }

  async #prepareRevision(
    context: CallContext,
    value: z.infer<typeof schemas.expected>,
    command: string,
    now: string,
    derive: (source: RevisionSource) => Promise<Derived>,
    operation: "edit" | "recalculate" = "edit",
  ) {
    const owner = this.#archive.document(value.id);
    owner.requireHead(value.expectedRevision);
    const header = owner.snapshot();
    const source = this.#archive.revision(value.id, value.expectedRevision);
    const { bytes, processing } = await derive({
      id: source.metadata.id,
      sha256: source.metadata.sha256,
      format: header.format,
      bytes: source.bytes,
    });
    const inspection = await this.#inspect(
      header.format,
      bytes,
      context.signal,
    );
    const next = revision({
      command,
      documentId: header.id,
      title: header.title,
      format: header.format,
      parent: source.metadata.id,
      operation,
      createdAt: now,
      bytes,
      provenance: source.metadata.provenance,
    });
    const document = owner.revise(value.expectedRevision, next);
    return () =>
      this.#archive.publish(
        document,
        next,
        bytes,
        inspection,
        value.expectedRevision,
        processing,
      );
  }

  async #prepareRender(
    context: CallContext,
    value: z.infer<typeof schemas.selected>,
    command: string,
    now: string,
  ) {
    const source = this.#archive.revision(value.id, value.revision);
    const rendered = await this.#engine.render(
      source.metadata.format,
      source.bytes,
      context.signal,
    );
    const files = rendered.files.map((file) => ({
      metadata: asset(command, file.filename, file.mime, file.bytes),
      bytes: file.bytes,
    }));
    const pdfs = files.filter((file) => file.metadata.mime === ASSET_MIME.pdf);
    if (
      pdfs.length !== 1 ||
      new Set(files.map((file) => file.metadata.filename)).size !== files.length
    )
      throw new Error(
        "deliverables: renderer requires one exact PDF and unique derivative filenames",
      );
    const report = renderReportSchema.parse({
      ...rendered.report,
      sourceRevision: value.revision,
      sourceSha256: source.metadata.sha256,
      pdfSha256: pdfs[0]!.metadata.sha256,
      pages: rendered.report.pages.map((page) => {
        const file = files.find(
          (file) =>
            file.metadata.filename === page.filename &&
            file.metadata.mime === ASSET_MIME.png,
        );
        if (!file)
          throw new Error("deliverables: rendered page lacks its exact PNG");
        return {
          number: page.number,
          assetId: file.metadata.id,
          sha256: file.metadata.sha256,
        };
      }),
    });
    if (
      !report.pages.length ||
      report.pages.some((page, index) => page.number !== index + 1)
    )
      throw new Error("deliverables: rendered page sequence is incomplete");
    const observation: Assessment = {
      id: digest(`assessment:${command}`),
      revision: value.revision,
      sourceSha256: source.metadata.sha256,
      createdAt: now,
      kind: "render",
      reportJSON: retainedJSON(report),
      assets: files.map((file) => file.metadata),
    };
    return () => this.#archive.assess(value.id, observation, files);
  }

  async #prepareReview(
    context: CallContext,
    value: z.infer<typeof schemas.review>,
    command: string,
    now: string,
  ) {
    const source = this.#archive.revision(value.id, value.revision);
    const rendered = this.#archive
      .assessments(value.id, value.revision)
      .find(
        (assessment) =>
          assessment.id === value.renderId && assessment.kind === "render",
      );
    if (!rendered)
      throw new Refusal(
        "render_not_found",
        "visual review requires an exact retained render for this revision",
      );
    const report = renderReportSchema.parse(JSON.parse(rendered.reportJSON));
    if (
      new Set(value.pages).size !== value.pages.length ||
      value.pages.some(
        (page) => !report.pages.some((entry) => entry.number === page),
      )
    )
      throw new Refusal(
        "invalid_review",
        "review pages must be unique and belong to the exact render",
      );
    const evidence = reviewReportSchema.parse({
      sourceRevision: value.revision,
      sourceSha256: source.metadata.sha256,
      renderId: value.renderId,
      pages: value.pages,
      verdict: value.verdict,
      notes: value.notes,
      reviewer: {
        origin: context.invocation!.origin,
        invocationId: context.invocation!.id,
      },
    });
    const observation: Assessment = {
      id: digest(`assessment:${command}`),
      revision: value.revision,
      sourceSha256: source.metadata.sha256,
      createdAt: now,
      kind: "review",
      reportJSON: retainedJSON(evidence),
      assets: [],
    };
    return () => this.#archive.assess(value.id, observation, []);
  }

  // The derivative is attached only if its source revision is still retained
  // when the command commits.
  #attach(
    value: z.infer<typeof schemas.selected>,
    metadata: Asset,
    bytes: Uint8Array,
  ) {
    return () => {
      this.#archive.revision(value.id, value.revision);
      return this.#archive.addAsset(value.id, value.revision, metadata, bytes);
    };
  }

  async #prepareExport(
    context: CallContext,
    value: z.infer<typeof schemas.export>,
    command: string,
    now: string,
  ) {
    const header = this.#archive.document(value.id).snapshot();
    const source = this.#archive.revision(value.id, value.revision);
    if (value.kind === "csv") {
      if (source.metadata.format !== "xlsx" || !value.sheetPart)
        throw new Refusal(
          "invalid_csv_source",
          "CSV export requires an XLSX revision and its inspected sheetPart",
        );
      const bytes = await this.#formats.csv(
        source.bytes,
        value.sheetPart,
        context.signal,
      );
      const metadata = asset(
        command,
        filename(header.title, "csv"),
        ASSET_MIME.csv,
        bytes,
      );
      return this.#attach(value, metadata, bytes);
    }
    if (value.sheetPart !== undefined)
      throw new Refusal(
        "invalid_export",
        "sheetPart is only meaningful for CSV export",
      );
    const native = () =>
      this.#archive.revision(value.id, value.revision).metadata.native;
    if (value.kind === "native") return native;
    const assessments = this.#archive.assessments(value.id, value.revision);
    const render = assessments.findLast(
      (observation) => observation.kind === "render",
    );
    if (value.kind === "pdf") {
      if (source.metadata.format === "pdf") return native;
      const pdf = render?.assets.find((file) => file.mime === ASSET_MIME.pdf);
      if (!pdf)
        throw new Refusal(
          "render_required",
          "render this exact native revision before exporting its PDF",
        );
      return () => this.#archive.asset(pdf.id).metadata;
    }
    const entries = new Map<string, Uint8Array>([
      [`native/${source.metadata.native.filename}`, source.bytes],
    ]);
    const manifest = {
      document: header,
      revision: source.metadata,
      inspection: source.inspection,
      processing: source.processing ?? null,
      assessments,
      exportedAt: now,
      meaning:
        "Native bytes are authoritative. Checks, renderings and attributed reviews apply only to the recorded revision. Author provenance is not independent verification.",
    };
    entries.set("evidence.json", Buffer.from(canonicalJSON(manifest)));
    if (render)
      for (const derivative of render.assets)
        entries.set(
          `render/${derivative.filename}`,
          this.#archive.asset(derivative.id).bytes,
        );
    const bytes = encodeArchive(entries, LIMITS.renderBytes);
    const metadata = asset(
      command,
      filename(header.title, "zip"),
      ASSET_MIME.zip,
      bytes,
    );
    return this.#attach(value, metadata, bytes);
  }
}

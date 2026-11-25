import { ASSET_MIME } from "../contracts/media";
import { canonicalJSON, type CallContext } from "@flame/backend-sdk/mcp";
import { z } from "zod";
import {
  schemas,
  type Asset,
  type Assessment,
  type Provenance,
} from "../contracts/capability";
import { LIMITS } from "../contracts/limits";
import { renderReportSchema, reviewReportSchema } from "../contracts/evidence";
import { Archive } from "./archive";
import { Document, digest, filename, revision, type Origin } from "./document";
import { Engine } from "./engine";
import { encodeArchive, type Format } from "./formats";
import { Formats } from "./format-process";
import { refusal } from "./refusal";

export function retainedJSON(value: unknown): string {
  const text = canonicalJSON(value);
  if (Buffer.byteLength(text) > LIMITS.reportBytes)
    throw refusal(
      "report_capacity",
      "inspection exceeds the retained evidence capacity",
    );
  return text;
}

export function asset(
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

interface InitialSource {
  title: string;
  format: Format;
  project?: string;
  provenance: Provenance;
  operation: "create" | "import" | "copy";
  bytes: Uint8Array;
  processing?: unknown;
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

// Builds each revision's bytes and evidence before the archive commits it; the
// returned publication runs inside the archive's command transaction.
export class RevisionPreparation {
  readonly #archive: Archive;
  readonly #engine: Engine;
  readonly #formats: Formats;

  constructor(archive: Archive, engine: Engine, formats: Formats) {
    this.#archive = archive;
    this.#engine = engine;
    this.#formats = formats;
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
      this.#archive.publish(
        document,
        initial,
        source.bytes,
        inspection,
        undefined,
        source.processing,
      );
  }

  async prepareCreate(
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

  async prepareImport(
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
      throw refusal(
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

  // A copy keeps the source's provenance with its content and records the
  // exact source it started from as processing evidence.
  async prepareCopy(
    context: CallContext,
    value: z.infer<typeof schemas.copy>,
    command: string,
    now: string,
  ) {
    const source = this.#archive.revision(value.id, value.revision);
    return this.#initial(context, command, now, {
      title: value.title,
      format: source.metadata.format,
      ...(value.project ? { project: value.project } : {}),
      provenance: source.metadata.provenance,
      operation: "copy",
      bytes: source.bytes,
      processing: {
        copiedDocument: value.id,
        copiedRevision: value.revision,
        sourceSha256: source.metadata.sha256,
      },
    });
  }

  async recalculate(
    source: RevisionSource,
    signal: AbortSignal,
  ): Promise<Derived> {
    if (source.format !== "xlsx")
      throw refusal(
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

  async prepareRevision(
    context: CallContext,
    value: z.infer<typeof schemas.expected>,
    command: string,
    now: string,
    derive: (source: RevisionSource) => Promise<Derived>,
    origin: Origin = { operation: "edit" },
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
      ...origin,
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

  // Restoring publishes the earlier revision's exact bytes on top of the
  // current one, so history keeps every revision in between.
  prepareRestore(
    context: CallContext,
    value: z.infer<typeof schemas.restore>,
    command: string,
    now: string,
  ) {
    return this.prepareRevision(
      context,
      value,
      command,
      now,
      async (source) => {
        const restored = this.#archive.revision(value.id, value.revision);
        if (restored.metadata.sha256 === source.sha256)
          throw refusal(
            "invalid_restore",
            "the revision's content is already current",
          );
        return { bytes: restored.bytes };
      },
      { operation: "restore", restored: value.revision },
    );
  }

  async prepareRender(
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

  async prepareReview(
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
      throw refusal(
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
      throw refusal(
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

  async prepareExport(
    context: CallContext,
    value: z.infer<typeof schemas.export>,
    command: string,
    now: string,
  ) {
    const header = this.#archive.document(value.id).snapshot();
    const source = this.#archive.revision(value.id, value.revision);
    if (value.kind === "csv") {
      if (source.metadata.format !== "xlsx" || !value.sheetPart)
        throw refusal(
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
      throw refusal(
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
        throw refusal(
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

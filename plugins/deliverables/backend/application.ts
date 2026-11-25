import { ASSET_MIME } from "../contracts/media";
import { canonicalJSON, type CallContext } from "@flame/backend-sdk/mcp";
import { toolCommandId } from "@flame/backend-sdk";
import { z } from "zod";
import {
  schemas,
  METHODS,
  type DocumentRead,
  type SuggestionValue,
  type ThreadMessage,
} from "../contracts/capability";
import { STAGED_OPERATIONS } from "../contracts/content";
import { LIMITS } from "../contracts/limits";
import { projectNumbers, type FrozenJSONValue } from "@frame/sdk/json";
import { engineCatalogSchema } from "../contracts/evidence";
import { Archive } from "./archive";
import { digest } from "./document";
import { Engine } from "./engine";
import { catalog, hasPNGSignature } from "./formats";
import { Formats } from "./format-process";
import { RevisionPreparation, retainedJSON } from "./revision-preparation";
import { refusal } from "./refusal";
import { Thread } from "./thread";

type Input = Record<string, unknown>;
export type Operation = keyof typeof METHODS;

export function decodeInput(schema: z.ZodType, value: unknown): unknown {
  // Envelope numbers are bounded integer counts and offsets. Content numbers
  // stay inside the format owner's original JSON string until format admission.
  // Frame admits method and tool input as frozen JSON.
  return schema.parse(projectNumbers(value as FrozenJSONValue));
}

function base64(value: string): Uint8Array {
  const result = Buffer.from(value, "base64");
  if (!result.byteLength || result.toString("base64") !== value)
    throw refusal(
      "invalid_chunk",
      "upload chunk requires nonempty canonical base64",
    );
  return result;
}

// Comments and suggestions annotate a document without changing a revision.
const ANNOTATIONS = [
  "comment",
  "reply",
  "resolve",
  "reopen",
  "suggest",
  "settle",
] as const;
type Annotation = (typeof ANNOTATIONS)[number];
const isAnnotation = (operation: Operation): operation is Annotation =>
  (ANNOTATIONS as readonly string[]).includes(operation);

export class Application {
  readonly #archive: Archive;
  readonly #engine: Engine;
  readonly #formats: Formats;
  readonly #preparation: RevisionPreparation;

  constructor(archive: Archive, engine: Engine, formats: Formats) {
    this.#archive = archive;
    this.#engine = engine;
    this.#formats = formats;
    this.#preparation = new RevisionPreparation(archive, engine, formats);
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
          throw refusal(
            "assessment_not_found",
            "assessment was not found for the exact native revision",
          );
        return result;
      }
      case "history": {
        const value = schemas.historyInput.parse(input);
        return this.#archive.history(value.id, value.before, value.limit);
      }
      case "suggestions": {
        const value = schemas.suggestionsInput.parse(input);
        return this.#archive.suggestions(
          value.id,
          value.status,
          value.before,
          value.limit,
        );
      }
      case "threads": {
        const value = schemas.threadsInput.parse(input);
        return this.#archive.threads(
          value.id,
          value.status,
          value.before,
          value.limit,
        );
      }
      case "slides":
        return schemas.slides.parse(
          await this.#projection(input, "pptx", "slide", (bytes) =>
            this.#formats.slides(bytes, signal),
          ),
        );
      case "sheets":
        return schemas.sheets.parse(
          await this.#projection(input, "xlsx", "sheet", (bytes) =>
            this.#formats.sheets(bytes, signal),
          ),
        );
      case "blocks":
        return schemas.blocks.parse(
          await this.#projection(input, "docx", "block", (bytes) =>
            this.#formats.blocks(bytes, signal),
          ),
        );
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
      throw refusal(
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
    if (isAnnotation(operation))
      return this.#annotate(context, operation, input, command, now);
    switch (operation) {
      case "create":
        return this.#preparation.prepareCreate(
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
      case "copy":
        return this.#preparation.prepareCopy(
          context,
          schemas.copy.parse(input),
          command,
          now,
        );
      case "import":
        return this.#preparation.prepareImport(
          context,
          schemas.import.parse(input),
          command,
          now,
        );
      case "edit": {
        const value = schemas.edit.parse(input);
        return this.#preparation.prepareRevision(
          context,
          value,
          command,
          now,
          (source) =>
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
        return this.#preparation.prepareRevision(
          context,
          schemas.expected.parse(input),
          command,
          now,
          (source) => this.#preparation.recalculate(source, context.signal),
          { operation: "recalculate" },
        );
      case "restore":
        return this.#preparation.prepareRestore(
          context,
          schemas.restore.parse(input),
          command,
          now,
        );
      case "render":
        return this.#preparation.prepareRender(
          context,
          schemas.selected.parse(input),
          command,
          now,
        );
      case "review":
        return this.#preparation.prepareReview(
          context,
          schemas.review.parse(input),
          command,
          now,
        );
      case "export":
        return this.#preparation.prepareExport(
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

  // A suggestion holds only edits an editor stages, and they must apply to the
  // revision it names; the trial result is discarded.
  async #suggest(
    context: CallContext,
    value: z.infer<typeof schemas.suggest>,
    command: string,
    now: string,
  ): Promise<() => unknown> {
    const source = this.#archive.revision(value.id, value.revision);
    const format = source.metadata.format;
    if (format === "pdf")
      throw refusal(
        "invalid_format",
        "suggestions require a PPTX, XLSX or DOCX revision",
      );
    const staged: readonly string[] = STAGED_OPERATIONS[format];
    const types = z
      .object({ operations: z.array(z.object({ type: z.string() })) })
      .parse(JSON.parse(value.operationsJSON))
      .operations.map((operation) => operation.type);
    const unstaged = types.filter((type) => !staged.includes(type));
    if (unstaged.length)
      throw refusal(
        "invalid_suggestion",
        `a ${format.toUpperCase()} suggestion holds only ${staged.join(", ")}; use edit for ${[...new Set(unstaged)].join(", ")}`,
      );
    await this.#formats.edit(
      format,
      source.bytes,
      value.operationsJSON,
      context.signal,
    );
    const suggestion: SuggestionValue = {
      id: digest(`suggestion:${command}`),
      documentId: value.id,
      revision: value.revision,
      note: value.note,
      operationsJSON: value.operationsJSON,
      author: {
        origin: context.invocation!.origin,
        invocationId: context.invocation!.id,
      },
      createdAt: now,
      status: "pending",
    };
    return () => this.#archive.addSuggestion(suggestion);
  }

  // Each transition applies to the thread as the publication transaction reads
  // it, so concurrent replies append instead of replacing each other.
  async #annotate(
    context: CallContext,
    operation: Annotation,
    input: Input,
    command: string,
    now: string,
  ): Promise<() => unknown> {
    const author = {
      origin: context.invocation!.origin,
      invocationId: context.invocation!.id,
    };
    const message = (body: string, revision?: string): ThreadMessage => ({
      id: digest(`message:${command}`),
      author,
      body,
      createdAt: now,
      ...(revision ? { revision } : {}),
    });
    switch (operation) {
      case "suggest":
        return this.#suggest(
          context,
          schemas.suggest.parse(input),
          command,
          now,
        );
      case "settle": {
        const value = schemas.settle.parse(input);
        return () =>
          this.#archive.settleSuggestion(
            value.id,
            value.suggestion,
            value.outcome,
            now,
          );
      }
      case "comment": {
        const { id, body, ...anchor } = schemas.comment.parse(input);
        return () => {
          this.#archive.revision(id, anchor.revision);
          return this.#archive.openThread(
            Thread.open(digest(`thread:${command}`), id, anchor, message(body)),
          );
        };
      }
      case "reply": {
        const value = schemas.reply.parse(input);
        return () => {
          if (value.revision) this.#archive.revision(value.id, value.revision);
          return this.#archive.saveThread(
            this.#archive
              .thread(value.id, value.thread)
              .reply(message(value.body, value.revision)),
          );
        };
      }
      default: {
        const value = schemas.threadTarget.parse(input);
        return () => {
          const thread = this.#archive.thread(value.id, value.thread);
          return this.#archive.saveThread(
            operation === "resolve" ? thread.resolve(now) : thread.reopen(now),
          );
        };
      }
    }
  }

  // Projections derive from one native revision on each read and are never stored.
  async #projection(
    input: unknown,
    format: "docx" | "pptx" | "xlsx",
    noun: string,
    project: (bytes: Uint8Array) => Promise<unknown>,
  ) {
    const value = schemas.readInput.parse(input);
    const header = this.#archive.document(value.id).snapshot();
    const source = this.#archive.revision(
      value.id,
      value.revision ?? header.head,
    );
    if (source.metadata.format !== format)
      throw refusal(
        "invalid_format",
        `${noun} projection requires a native ${format.toUpperCase()} revision`,
      );
    const layoutJSON = canonicalJSON(await project(source.bytes));
    if (Buffer.byteLength(layoutJSON) > LIMITS.reportBytes)
      throw refusal(
        "report_capacity",
        `${noun} projection exceeds its report capacity`,
      );
    return {
      document: header,
      revision: source.metadata.id,
      sourceSha256: source.metadata.sha256,
      layoutJSON,
    };
  }
}

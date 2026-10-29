import { createHash } from "node:crypto";
import { Refusal, canonicalJSON } from "@qimu/node";
import {
  schemas,
  type Header,
  type Revision,
  type Format,
  type Provenance,
} from "../contracts/capability";

export function digest(bytes: Uint8Array | string): string {
  return createHash("sha256").update(bytes).digest("hex");
}

export const MIME: Readonly<Record<Format, string>> = Object.freeze({
  docx: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
  xlsx: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
  pptx: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
  pdf: "application/pdf",
});

export function filename(title: string, extension: string): string {
  const cleaned = title
    .replace(/[\p{Cc}<>:"/\\|?*]/gu, "_")
    .replace(/[. ]+$/u, "");
  let stem = "";
  for (const character of cleaned) {
    if (Buffer.byteLength(stem + character) > 180) break;
    stem += character;
  }
  if (!stem || /^(?:CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(?:\.|$)/iu.test(stem))
    stem = `deliverable${stem ? `-${stem}` : ""}`;
  return `${stem}.${extension}`;
}

export function revision(input: {
  command: string;
  documentId: string;
  title: string;
  format: Format;
  parent?: string;
  operation: Revision["operation"];
  createdAt: string;
  bytes: Uint8Array;
  provenance: Provenance;
}): Revision {
  const hash = digest(input.bytes);
  const id = digest(
    canonicalJSON({
      command: input.command,
      documentId: input.documentId,
      parent: input.parent ?? null,
      hash,
    }),
  );
  return schemas.revision.parse({
    id,
    documentId: input.documentId,
    ...(input.parent ? { parent: input.parent } : {}),
    createdAt: input.createdAt,
    operation: input.operation,
    format: input.format,
    sha256: hash,
    size: input.bytes.byteLength,
    provenance: input.provenance,
    native: {
      id,
      filename: filename(input.title, input.format),
      mime: MIME[input.format],
      size: input.bytes.byteLength,
      sha256: hash,
    },
  });
}

export class Document {
  readonly #header: Header;

  constructor(value: Header) {
    this.#header = Object.freeze(schemas.header.parse(value));
  }

  static create(title: string, initial: Revision): Document {
    if (initial.parent || !["create", "import"].includes(initial.operation))
      throw new Error(
        "deliverables: initial revision requires a creation or import",
      );
    return new Document({
      id: initial.documentId,
      title,
      format: initial.format,
      head: initial.id,
      createdAt: initial.createdAt,
    });
  }

  snapshot(): Header {
    return { ...this.#header };
  }

  requireHead(expected: string): void {
    if (this.#header.head !== expected)
      throw new Refusal(
        "revision_conflict",
        "the deliverable changed; inspect its current revision before editing",
      );
  }

  revise(expected: string, next: Revision): Document {
    this.requireHead(expected);
    if (
      next.documentId !== this.#header.id ||
      next.parent !== expected ||
      next.format !== this.#header.format ||
      !["edit", "recalculate"].includes(next.operation) ||
      next.id === expected
    )
      throw new Error(
        "deliverables: revision ancestry or format differs from its document",
      );
    return new Document({ ...this.#header, head: next.id });
  }
}

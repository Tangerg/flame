interface ComposerDraftImage {
  readonly mime: string;
  readonly data: string;
  readonly name?: string;
}

// The producer wrote the label people see and the text the model receives; the
// draft only carries a reference until it is sent or removed.
export interface ComposerReference {
  readonly label: string;
  readonly text: string;
}

export interface ComposerDraftReference extends ComposerReference {
  readonly id: string;
}

export interface ComposerDraftInput {
  text: string;
  images?: readonly ComposerDraftImage[];
  references?: readonly ComposerReference[];
}

export interface ComposerImage extends ComposerDraftImage {
  readonly id: string;
}

export type ComposerImageAttachment =
  | Readonly<ComposerImage & { status: "ready" }>
  | { readonly status: "pending" | "failed"; readonly id: string; readonly name: string };

export type ImageReadResult =
  { status: "ready"; image: Omit<ComposerImage, "id"> } | { status: "failed" };

export interface PastedText {
  readonly id: string;
  readonly text: string;
  readonly lines: number;
}

const PARAGRAPH_BREAK = "\n\n";

export function joinDraftParts(parts: readonly string[]): string {
  return parts.filter(Boolean).join(PARAGRAPH_BREAK);
}

// Quoting keeps a path with whitespace inside one prompt token.
export function formatFileReference(path: string): string {
  if (!/\s/.test(path) && !path.startsWith('"')) return `@${path}`;
  return `@"${path.replace(/["\\]/g, (char) => `\\${char}`)}"`;
}

export type DraftSegment =
  | { readonly kind: "text"; readonly text: string }
  | { readonly kind: "file"; readonly path: string };

export class DraftContent {
  private static readonly EMPTY = new DraftContent([]);

  readonly text: string;

  private constructor(readonly segments: readonly DraftSegment[]) {
    this.text = segments
      .map((segment) =>
        segment.kind === "text" ? segment.text : formatFileReference(segment.path),
      )
      .join("");
  }

  static empty(): DraftContent {
    return DraftContent.EMPTY;
  }

  static fromText(text: string): DraftContent {
    return DraftContent.of([{ kind: "text", text }]);
  }

  static of(segments: readonly DraftSegment[]): DraftContent {
    const normalized: DraftSegment[] = [];
    for (const segment of segments) {
      if (segment.kind === "file") {
        if (!segment.path) throw new Error("file reference has no path");
        normalized.push({ kind: "file", path: segment.path });
        continue;
      }
      if (!segment.text) continue;
      const last = normalized.at(-1);
      if (last?.kind === "text")
        normalized[normalized.length - 1] = { kind: "text", text: last.text + segment.text };
      else normalized.push({ kind: "text", text: segment.text });
    }
    return normalized.length ? new DraftContent(normalized) : DraftContent.EMPTY;
  }

  get isEmpty(): boolean {
    return this.segments.length === 0;
  }

  equals(other: DraftContent): boolean {
    return (
      this.segments.length === other.segments.length &&
      this.segments.every((segment, index) => {
        const peer = other.segments[index]!;
        return segment.kind === "text"
          ? peer.kind === "text" && peer.text === segment.text
          : peer.kind === "file" && peer.path === segment.path;
      })
    );
  }

  append(text: string): DraftContent {
    if (!text) return this;
    const base = this.trimEnd();
    return base.isEmpty ? DraftContent.fromText(text) : base.concat(PARAGRAPH_BREAK + text);
  }

  prepend(before: DraftContent): DraftContent {
    if (before.isEmpty) return this;
    if (this.isEmpty) return before;
    return DraftContent.of([
      ...before.segments,
      { kind: "text", text: PARAGRAPH_BREAK },
      ...this.segments,
    ]);
  }

  startFileReference(): DraftContent {
    return this.isEmpty ? DraftContent.fromText("@") : this.trimEnd().concat(" @");
  }

  startSlashCommand(): DraftContent {
    if (this.text.trim()) throw new Error("a slash command must start a blank draft");
    return DraftContent.fromText("/");
  }

  private trimEnd(): DraftContent {
    const last = this.segments.at(-1);
    if (last?.kind !== "text") return this;
    return DraftContent.of([
      ...this.segments.slice(0, -1),
      { kind: "text", text: last.text.trimEnd() },
    ]);
  }

  private concat(text: string): DraftContent {
    return DraftContent.of([...this.segments, { kind: "text", text }]);
  }
}

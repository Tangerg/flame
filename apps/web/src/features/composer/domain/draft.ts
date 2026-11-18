interface ComposerDraftImage {
  readonly mime: string;
  readonly data: string;
  readonly name?: string;
}

export interface ComposerDraftInput {
  text: string;
  images?: readonly ComposerDraftImage[];
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

export function joinDraftParts(parts: readonly string[]): string {
  return parts.filter(Boolean).join("\n\n");
}

// Quoting keeps a path with whitespace inside one prompt token.
export function formatFileReference(path: string): string {
  if (!/\s/.test(path) && !path.startsWith('"')) return `@${path}`;
  return `@"${path.replace(/["\\]/g, (char) => `\\${char}`)}"`;
}

export type DraftSegment =
  | { readonly kind: "text"; readonly text: string }
  | { readonly kind: "file"; readonly path: string };

/**
 * The draft document: text plus file references accepted from candidates. Typed
 * `@path` text stays text. The prompt text is a projection; Runtime input is text.
 */
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
    return base.isEmpty ? DraftContent.fromText(text) : base.concat(`\n\n${text}`);
  }

  // A trailing "@" token opens file candidates; accepting one replaces it with a reference.
  startFileReference(): DraftContent {
    return this.isEmpty ? DraftContent.fromText("@") : this.trimEnd().concat(" @");
  }

  // A command must open the message, so only a blank draft can start one.
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

import {
  DraftContent,
  type ComposerImage,
  type ComposerImageAttachment,
  type ImageReadResult,
  type PastedText,
} from "@/features/composer/domain/draft";
import { countLines } from "@/features/composer/domain/largePaste";
const SCRATCH_SESSION_ID = "";

const HISTORY_CAP = 50;

export interface ComposerDraftText {
  content: DraftContent;
  pastes: readonly Pick<PastedText, "id" | "text">[];
}

export class ComposerDraft {
  private static readonly EMPTY = new ComposerDraft(
    DraftContent.empty(),
    Object.freeze([]),
    Object.freeze([]),
  );

  readonly images: readonly ComposerImage[];

  private constructor(
    readonly content: DraftContent,
    readonly imageAttachments: readonly ComposerImageAttachment[],
    readonly pastes: readonly PastedText[],
  ) {
    this.images = imageAttachments.flatMap((attachment) =>
      attachment.status === "ready"
        ? [
            {
              id: attachment.id,
              mime: attachment.mime,
              data: attachment.data,
              name: attachment.name,
            },
          ]
        : [],
    );
  }

  static empty(): ComposerDraft {
    return ComposerDraft.EMPTY;
  }

  static restore(content: DraftContent): ComposerDraft {
    return new ComposerDraft(content, [], []);
  }

  get value(): string {
    return this.content.text;
  }

  get attachmentsReady(): boolean {
    return this.imageAttachments.every((attachment) => attachment.status === "ready");
  }

  hasSameText(other: ComposerDraft): boolean {
    return (
      this.content.equals(other.content) &&
      this.pastes.length === other.pastes.length &&
      this.pastes.every(
        (paste, index) =>
          paste.id === other.pastes[index]?.id && paste.text === other.pastes[index]?.text,
      )
    );
  }

  hasSameAttachments(other: ComposerDraft): boolean {
    return (
      this.imageAttachments.length === other.imageAttachments.length &&
      this.imageAttachments.every((image, index) => image === other.imageAttachments[index])
    );
  }

  withContent(content: DraftContent): ComposerDraft {
    return content.equals(this.content)
      ? this
      : new ComposerDraft(content, this.imageAttachments, this.pastes);
  }

  hydrateText(draft: ComposerDraft): ComposerDraft {
    return new ComposerDraft(draft.content, this.imageAttachments, draft.pastes);
  }

  withImages(images: readonly ComposerImage[]): ComposerDraft {
    return new ComposerDraft(
      this.content,
      images.map((image) => ({ ...image, status: "ready" })),
      this.pastes,
    );
  }

  appendImages(images: readonly ComposerImage[]): ComposerDraft {
    return new ComposerDraft(
      this.content,
      [
        ...this.imageAttachments,
        ...images.map((image) => ({ ...image, status: "ready" as const })),
      ],
      this.pastes,
    );
  }

  beginImageRead(images: readonly { id: string; name: string }[]): ComposerDraft {
    return new ComposerDraft(
      this.content,
      [
        ...this.imageAttachments,
        ...images.map((image) => ({ ...image, status: "pending" as const })),
      ],
      this.pastes,
    );
  }

  settleImageRead(id: string, result: ImageReadResult): ComposerDraft {
    const pending = this.imageAttachments.find((attachment) => attachment.id === id);
    if (!pending || pending.status !== "pending") return this;
    const settled: ComposerImageAttachment =
      result.status === "ready"
        ? { ...result.image, id, name: pending.name, status: "ready" }
        : { id, name: pending.name, status: "failed" };
    return new ComposerDraft(
      this.content,
      this.imageAttachments.map((attachment) => (attachment === pending ? settled : attachment)),
      this.pastes,
    );
  }

  removeImage(id: string): ComposerDraft {
    if (!this.imageAttachments.some((attachment) => attachment.id === id)) return this;
    return new ComposerDraft(
      this.content,
      this.imageAttachments.filter((attachment) => attachment.id !== id),
      this.pastes,
    );
  }

  withPastes(pastes: readonly PastedText[]): ComposerDraft {
    return new ComposerDraft(this.content, this.imageAttachments, pastes);
  }

  editPaste(id: string, text: string): ComposerDraft {
    if (!text.trim()) return this.withPastes(this.pastes.filter((paste) => paste.id !== id));
    return this.withPastes(
      this.pastes.map((paste) => (paste.id === id ? { id, text, lines: countLines(text) } : paste)),
    );
  }

  restorePaste(id: string, text: string): ComposerDraft {
    if (!this.pastes.some((paste) => paste.id === id)) return this;
    return new ComposerDraft(
      text.trim() ? this.content.append(text) : this.content,
      this.imageAttachments,
      this.pastes.filter((paste) => paste.id !== id),
    );
  }
}

type Recall =
  { readonly active: false } | { readonly active: true; at: number; saved: DraftContent };

const NOT_RECALLING: Recall = { active: false };

export class Composer {
  private constructor(
    private readonly drafts: ReadonlyMap<string, ComposerDraft>,
    private readonly rings: ReadonlyMap<string, readonly string[]>,
    readonly activeSessionId: string,
    private readonly recall: Recall,
  ) {}

  static empty(): Composer {
    return new Composer(new Map(), new Map(), SCRATCH_SESSION_ID, NOT_RECALLING);
  }

  static restoreDrafts(texts: ReadonlyMap<string, ComposerDraftText>): Composer {
    const drafts = new Map<string, ComposerDraft>();
    for (const [sessionId, text] of texts) {
      drafts.set(
        sessionId,
        ComposerDraft.restore(text.content).withPastes(
          text.pastes.map(({ id, text }) => ({ id, text, lines: countLines(text) })),
        ),
      );
    }
    return new Composer(drafts, new Map(), SCRATCH_SESSION_ID, NOT_RECALLING);
  }

  get draft(): ComposerDraft {
    return this.drafts.get(this.activeSessionId) ?? ComposerDraft.empty();
  }

  get isRecalling(): boolean {
    return this.recall.active;
  }

  durableDraftTexts(): Map<string, ComposerDraftText> {
    const texts = new Map<string, ComposerDraftText>();
    for (const [sessionId, draft] of this.drafts) {
      if (draft.content.isEmpty && draft.pastes.length === 0) continue;
      // Flattening a paste into the content could turn authored text into a slash command.
      texts.set(sessionId, {
        content: draft.content,
        pastes: draft.pastes.map(({ id, text }) => ({ id, text })),
      });
    }
    return texts;
  }

  hydrateDrafts(
    texts: ReadonlyMap<string, ComposerDraftText>,
    touched: ReadonlySet<string>,
  ): Composer {
    const restored = Composer.restoreDrafts(texts);
    const drafts = new Map(this.drafts);
    for (const [sessionId, draft] of restored.drafts) {
      if (!touched.has(sessionId)) {
        const current = drafts.get(sessionId);
        drafts.set(sessionId, current ? current.hydrateText(draft) : draft);
      }
    }
    return new Composer(drafts, this.rings, this.activeSessionId, this.recall);
  }

  // An unchanged draft is not an edit, so restoring recalled text into the editor keeps the recall.
  edit(change: (draft: ComposerDraft) => ComposerDraft): Composer {
    const draft = change(this.draft);
    return draft === this.draft ? this : this.replaceDraft(draft, NOT_RECALLING);
  }

  clear(): Composer {
    return this.replaceDraft(ComposerDraft.empty(), NOT_RECALLING);
  }

  settleImageRead(sessionId: string, id: string, result: ImageReadResult): Composer {
    const draft = this.drafts.get(sessionId);
    if (!draft) return this;
    const settled = draft.settleImageRead(id, result);
    if (settled === draft) return this;
    return new Composer(
      new Map(this.drafts).set(sessionId, settled),
      this.rings,
      this.activeSessionId,
      this.recall,
    );
  }

  consumeDraft(sessionId: string, draft: ComposerDraft, historyText?: string): Composer {
    if (!draft.attachmentsReady || !this.drafts.has(sessionId)) return this;
    const recorded = historyText ? this.record(historyText, sessionId) : this;
    if (this.drafts.get(sessionId) !== draft) return recorded;
    return new Composer(
      new Map(this.drafts).set(sessionId, ComposerDraft.empty()),
      recorded.rings,
      this.activeSessionId,
      sessionId === this.activeSessionId ? NOT_RECALLING : this.recall,
    );
  }

  activate(sessionId: string): Composer {
    if (sessionId === this.activeSessionId) return this;
    return new Composer(this.drafts, this.rings, sessionId, NOT_RECALLING);
  }

  discardSession(sessionId: string): Composer {
    if (sessionId === SCRATCH_SESSION_ID) return this;
    const drafts = new Map(this.drafts);
    const rings = new Map(this.rings);
    drafts.delete(sessionId);
    rings.delete(sessionId);
    return new Composer(drafts, rings, this.activeSessionId, NOT_RECALLING);
  }

  record(text: string, sessionId = this.activeSessionId): Composer {
    const value = text.trim();
    if (!value) return this;
    const ring = this.rings.get(sessionId) ?? [];
    const recorded =
      ring[ring.length - 1] === value
        ? this
        : this.withRing([...ring, value].slice(-HISTORY_CAP), sessionId);
    return sessionId === this.activeSessionId ? recorded.withRecall(NOT_RECALLING) : recorded;
  }

  recallOlder(): Composer | null {
    const ring = this.ring();
    if (ring.length === 0) return null;
    const at = this.recall.active ? Math.min(this.recall.at + 1, ring.length - 1) : 0;
    const saved = this.recall.active ? this.recall.saved : this.draft.content;
    return this.replaceDraft(
      this.draft.withContent(DraftContent.fromText(ring[ring.length - 1 - at]!)),
      {
        active: true,
        at,
        saved,
      },
    );
  }

  recallNewer(): Composer | null {
    if (!this.recall.active) return null;
    const at = this.recall.at - 1;
    const ring = this.ring();
    if (at < 0) return this.replaceDraft(this.draft.withContent(this.recall.saved), NOT_RECALLING);
    return this.replaceDraft(
      this.draft.withContent(DraftContent.fromText(ring[ring.length - 1 - at]!)),
      {
        ...this.recall,
        at,
      },
    );
  }

  private ring(): readonly string[] {
    return this.rings.get(this.activeSessionId) ?? [];
  }

  private withRing(ring: readonly string[], sessionId: string): Composer {
    return new Composer(
      this.drafts,
      new Map(this.rings).set(sessionId, ring),
      this.activeSessionId,
      this.recall,
    );
  }

  private withRecall(recall: Recall): Composer {
    return new Composer(this.drafts, this.rings, this.activeSessionId, recall);
  }

  private replaceDraft(draft: ComposerDraft, recall: Recall): Composer {
    return new Composer(
      new Map(this.drafts).set(this.activeSessionId, draft),
      this.rings,
      this.activeSessionId,
      recall,
    );
  }
}

import { type ComposerImage, type PastedText, joinDraftParts } from "./draft";
import { countLines } from "./largePaste";

export const SCRATCH_SESSION_ID = "";

const HISTORY_CAP = 50;

export interface ComposerDraftText {
  value: string;
  pastes: readonly Pick<PastedText, "id" | "text">[];
}

export class ComposerDraft {
  private static readonly EMPTY = new ComposerDraft("", Object.freeze([]), Object.freeze([]));

  private constructor(
    readonly value: string,
    readonly images: readonly ComposerImage[],
    readonly pastes: readonly PastedText[],
  ) {}

  static empty(): ComposerDraft {
    return ComposerDraft.EMPTY;
  }

  static restoreText(value: string): ComposerDraft {
    return new ComposerDraft(value, [], []);
  }

  withValue(value: string): ComposerDraft {
    return value === this.value ? this : new ComposerDraft(value, this.images, this.pastes);
  }

  withImages(images: readonly ComposerImage[]): ComposerDraft {
    return new ComposerDraft(this.value, images, this.pastes);
  }

  withPastes(pastes: readonly PastedText[]): ComposerDraft {
    return new ComposerDraft(this.value, this.images, pastes);
  }

  editPaste(id: string, text: string): ComposerDraft {
    if (!text.trim()) return this.withPastes(this.pastes.filter((paste) => paste.id !== id));
    return this.withPastes(
      this.pastes.map((paste) => (paste.id === id ? { id, text, lines: countLines(text) } : paste)),
    );
  }

  restorePaste(id: string): ComposerDraft {
    const paste = this.pastes.find((candidate) => candidate.id === id);
    if (!paste) return this;
    return new ComposerDraft(
      joinDraftParts([this.value.trimEnd(), paste.text]),
      this.images,
      this.pastes.filter((candidate) => candidate.id !== id),
    );
  }
}

type Recall = { readonly active: false } | { readonly active: true; at: number; saved: string };

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
        ComposerDraft.restoreText(text.value).withPastes(
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
      if (!draft.value && draft.pastes.length === 0) continue;
      // Flattening a paste into value could turn authored content into a slash command.
      texts.set(sessionId, {
        value: draft.value,
        pastes: draft.pastes.map(({ id, text }) => ({ id, text })),
      });
    }
    return texts;
  }

  edit(change: (draft: ComposerDraft) => ComposerDraft): Composer {
    return this.replaceDraft(change(this.draft), NOT_RECALLING);
  }

  clear(): Composer {
    return this.replaceDraft(ComposerDraft.empty(), NOT_RECALLING);
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

  record(text: string): Composer {
    const value = text.trim();
    if (!value) return this;
    const ring = this.ring();
    if (ring[ring.length - 1] === value) return this.withRecall(NOT_RECALLING);
    return this.withRing([...ring, value].slice(-HISTORY_CAP)).withRecall(NOT_RECALLING);
  }

  recallOlder(): Composer | null {
    const ring = this.ring();
    if (ring.length === 0) return null;
    const at = this.recall.active ? Math.min(this.recall.at + 1, ring.length - 1) : 0;
    const saved = this.recall.active ? this.recall.saved : this.draft.value;
    return this.replaceDraft(this.draft.withValue(ring[ring.length - 1 - at]!), {
      active: true,
      at,
      saved,
    });
  }

  recallNewer(): Composer | null {
    if (!this.recall.active) return null;
    const at = this.recall.at - 1;
    const ring = this.ring();
    if (at < 0) return this.replaceDraft(this.draft.withValue(this.recall.saved), NOT_RECALLING);
    return this.replaceDraft(this.draft.withValue(ring[ring.length - 1 - at]!), {
      ...this.recall,
      at,
    });
  }

  private ring(): readonly string[] {
    return this.rings.get(this.activeSessionId) ?? [];
  }

  private withRing(ring: readonly string[]): Composer {
    return new Composer(
      this.drafts,
      new Map(this.rings).set(this.activeSessionId, ring),
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

import type { ComposerImage, PastedText } from "./draft";

/** The welcome screen has no Session, and what is typed there must survive reaching one. */
export const SCRATCH_SESSION_ID = "";

const HISTORY_CAP = 50;

/**
 * What one conversation has half-written. Immutable: the store swaps whole drafts, so a
 * selector that returned `images` last render returns the same array this render unless
 * the images actually changed.
 */
export class ComposerDraft {
  private static readonly EMPTY = new ComposerDraft("", Object.freeze([]), Object.freeze([]));

  private constructor(
    readonly value: string,
    readonly images: readonly ComposerImage[],
    readonly pastes: readonly PastedText[],
  ) {}

  /** One instance, because `Composer.draft` answers it for every session that has not been
   *  typed into. A fresh object there would hand selectors a new `images` array on every
   *  store notification, which `Object.is` never matches — a re-render on every keystroke
   *  in any other session. */
  static empty(): ComposerDraft {
    return ComposerDraft.EMPTY;
  }

  /** Only the text is durable; images and pastes are heavy and meant to be sent at once. */
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
}

/**
 * Where the input ring is being read from, as a closed state.
 *
 * This was an index with `-1` meaning "not recalling" plus a separate saved draft that was
 * only meaningful while it was not `-1`. Four call sites had to remember to reset both.
 */
type Recall = { readonly active: false } | { readonly active: true; at: number; saved: string };

const NOT_RECALLING: Recall = { active: false };

/**
 * The composer aggregate: every session's draft, every session's input ring, and which
 * session is being edited.
 *
 * One root because the invariants span all three. `value` used to be mirrored beside the
 * archive it was a copy of, and leaving recall on an edit was four separate assignments —
 * both are structural here: the active draft is DERIVED, and every mutation but recall's
 * own returns to `NOT_RECALLING`.
 */
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

  static restoreDrafts(texts: ReadonlyMap<string, string>): Composer {
    const drafts = new Map<string, ComposerDraft>();
    for (const [sessionId, value] of texts) drafts.set(sessionId, ComposerDraft.restoreText(value));
    return new Composer(drafts, new Map(), SCRATCH_SESSION_ID, NOT_RECALLING);
  }

  get draft(): ComposerDraft {
    return this.drafts.get(this.activeSessionId) ?? ComposerDraft.empty();
  }

  get isRecalling(): boolean {
    return this.recall.active;
  }

  /** Durable text per session, for the persisted half. Empty drafts are not worth storing. */
  durableDraftTexts(): Map<string, string> {
    const texts = new Map<string, string>();
    for (const [sessionId, draft] of this.drafts) {
      if (draft.value) texts.set(sessionId, draft.value);
    }
    return texts;
  }

  edit(change: (draft: ComposerDraft) => ComposerDraft): Composer {
    return this.replaceDraft(change(this.draft), NOT_RECALLING);
  }

  clear(): Composer {
    return this.replaceDraft(ComposerDraft.empty(), NOT_RECALLING);
  }

  /** Returns the same instance when already there, so the caller need not compare ids. */
  activate(sessionId: string): Composer {
    if (sessionId === this.activeSessionId) return this;
    return new Composer(this.drafts, this.rings, sessionId, NOT_RECALLING);
  }

  /** The scratch draft and the session being edited survive regardless of the live set. */
  prune(liveSessionIds: ReadonlySet<string>): Composer {
    const keep = (sessionId: string) =>
      sessionId === SCRATCH_SESSION_ID ||
      sessionId === this.activeSessionId ||
      liveSessionIds.has(sessionId);
    return new Composer(
      retain(this.drafts, keep),
      retain(this.rings, keep),
      this.activeSessionId,
      this.recall,
    );
  }

  /** Blank text and an immediate repeat are not history. */
  record(text: string): Composer {
    const value = text.trim();
    if (!value) return this;
    const ring = this.ring();
    if (ring[ring.length - 1] === value) return this.withRecall(NOT_RECALLING);
    return this.withRing([...ring, value].slice(-HISTORY_CAP)).withRecall(NOT_RECALLING);
  }

  /** Null when there is nothing older, so a keymap can fall through to cursor movement. */
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

  /** Null when not recalling. Stepping past the newest entry restores what was typed. */
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

function retain<T>(
  source: ReadonlyMap<string, T>,
  keep: (key: string) => boolean,
): ReadonlyMap<string, T> {
  const kept = new Map<string, T>();
  for (const [key, value] of source) if (keep(key)) kept.set(key, value);
  return kept;
}

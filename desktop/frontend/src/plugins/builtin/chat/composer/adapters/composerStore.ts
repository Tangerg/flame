import { nanoid } from "nanoid";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { fileToInputImage } from "@/plugins/builtin/chat/composer/public/input";
import { countLines } from "@/plugins/builtin/chat/composer/public/largePaste";
import { t } from "@/lib/i18n";
import { discardOlderVersions } from "@/lib/persistedStore";
import { notifyError } from "@/plugins/sdk";
import type { ComposerImage, PastedText } from "../domain/draft";
import type { ComposerModelPreference } from "../application/ports/state";
import {
  emptyComposerDraft,
  loadComposerDraft,
  mirrorComposerDraft,
  nextComposerHistory,
  previousComposerHistory,
  pruneComposerArchives,
  pushComposerHistory,
  type ComposerDraftArchive,
  type ComposerHistoryArchive,
} from "../domain/draftArchive";
import { parsePersistedComposerDrafts, persistedComposerDrafts } from "./persistedDrafts";

// Drafts are kept PER SESSION so switching tabs never shows — or clobbers — another
// conversation's half-written message. Only `value` is durable; staged images and pastes
// are heavy and meant to be sent immediately, so they are dropped on reload.

interface ComposerState {
  // MIRROR the active session's draft so plain selectors keep working; `drafts` is the
  // per-session archive, swapped into the mirror by `loadSession`.
  value: string;
  images: ComposerImage[];
  pastes: PastedText[];
  // GLOBAL, not mirrored: an explicit model pick is not per-conversation work and carries
  // across sessions.
  modelPreference: ComposerModelPreference;
  /** Keyed by sessionId; "" is the no-session scratch draft on the welcome screen. */
  drafts: ComposerDraftArchive;
  activeSid: string;
  /** In-memory only — the transcript already survives reload; this is the input ring. */
  history: ComposerHistoryArchive;
  /** -1 = not navigating, 0 = most recent, 1 = next older … */
  histIndex: number;
  /** Saved when recall begins, so stepping past the newest entry restores it. */
  histDraft: string;
}

interface ComposerActions {
  setValue: (v: string) => void;
  setModel: (preference: ComposerModelPreference) => void;
  clear: () => void;
  addImages: (imgs: Omit<ComposerImage, "id">[]) => void;
  /** Fire-and-forget and per-file tolerant. Dropped entirely if the composer is cleared or
   *  the active session switches mid-decode, so a late image cannot leak into the next
   *  message or another conversation's draft. */
  addImageFiles: (files: File[]) => void;
  removeImage: (id: string) => void;
  /** Keeps a large blob out of the textarea; re-inlined into the message on send. */
  addPaste: (text: string) => void;
  removePaste: (id: string) => void;
  /** Mutations keep `drafts[activeSid]` current, so this only LOADS the target. */
  loadSession: (sid: string) => void;
  /** Bounds the archive when a session tab closes; the scratch and active drafts survive. */
  pruneDrafts: (liveSids: Set<string>) => void;
  pushHistory: (text: string) => void;
  /** False when there is no history to recall, so the key falls through to cursor movement. */
  historyPrev: () => boolean;
  /** False when not currently navigating history. */
  historyNext: () => boolean;
}

const HISTORY_CAP = 50;

export const useComposerStore = create<ComposerState & ComposerActions>()(
  persist(
    (set, get) => {
      // Replaced on every clear(). `addImageFiles` captures it when its decode starts and
      // drops the result if it advanced, so an image still decoding at submit time is
      // discarded rather than leaking into the NEXT message.
      let stagingLease: object = {};
      return {
        value: "",
        images: [],
        pastes: [],
        modelPreference: { kind: "session" },
        drafts: {},
        activeSid: "",
        history: {},
        histIndex: -1,
        histDraft: "",
        setValue: (value) => set((s) => ({ ...mirrorComposerDraft(s, { value }), histIndex: -1 })),
        setModel: (modelPreference) => set({ modelPreference }),
        clear: () => {
          stagingLease = {};
          set((s) => ({ ...mirrorComposerDraft(s, emptyComposerDraft()), histIndex: -1 }));
        },
        addImages: (imgs) =>
          set((s) =>
            mirrorComposerDraft(s, {
              images: [...s.images, ...imgs.map((i) => ({ id: nanoid(), ...i }))],
            }),
          ),
        addImageFiles: (files) => {
          const lease = stagingLease;
          const sid = get().activeSid;
          // `allSettled`, not `all`: one unreadable file must not discard the batch, and
          // the chain must never reject (there is no global rejection handler).
          void Promise.allSettled(files.map(fileToInputImage)).then((results) => {
            // `addImages` writes the CURRENT activeSid's mirror, so a stale lease or a
            // switched session must drop the result entirely.
            if (lease !== stagingLease || get().activeSid !== sid) return;
            const ok = results.flatMap((r) => (r.status === "fulfilled" ? [r.value] : []));
            if (ok.length > 0) get().addImages(ok);
            const failed = results.length - ok.length;
            if (failed > 0)
              notifyError(
                failed > 1
                  ? t("composer.error.readImages", { count: failed })
                  : t("composer.error.readImage"),
                {
                  source: "composer",
                },
              );
          });
        },
        removeImage: (id) =>
          set((s) => mirrorComposerDraft(s, { images: s.images.filter((i) => i.id !== id) })),
        addPaste: (text) =>
          set((s) =>
            mirrorComposerDraft(s, {
              pastes: [...s.pastes, { id: nanoid(), text, lines: countLines(text) }],
            }),
          ),
        removePaste: (id) =>
          set((s) => mirrorComposerDraft(s, { pastes: s.pastes.filter((p) => p.id !== id) })),
        loadSession: (sid) =>
          set((s) => {
            return loadComposerDraft(s, sid) ?? s;
          }),
        pruneDrafts: (liveSids) =>
          set((s) => {
            return pruneComposerArchives(s, liveSids);
          }),
        pushHistory: (text) =>
          set((s) => {
            return pushComposerHistory(s, text, HISTORY_CAP) ?? s;
          }),
        historyPrev: () => {
          const s = get();
          const next = previousComposerHistory(s);
          if (!next) return false;
          set((st) => ({
            ...mirrorComposerDraft(st, { value: next.value }),
            histIndex: next.histIndex,
            histDraft: next.histDraft,
          }));
          return true;
        },
        historyNext: () => {
          const s = get();
          const next = nextComposerHistory(s);
          if (!next) return false;
          set((st) => ({
            ...mirrorComposerDraft(st, { value: next.value }),
            histIndex: next.histIndex,
          }));
          return true;
        },
      };
    },
    {
      name: "flame.composer",
      storage: createJSONStorage(() => localStorage),
      version: 1,
      migrate: discardOlderVersions,
      // Text-only: `value` rehydrates from `drafts` via the cold-start loadSession below,
      // images are transient, and the model fallback comes from the active Session.
      partialize: (s) => ({ drafts: persistedComposerDrafts(s.drafts) }),
      merge: (persisted, current) => {
        const drafts = parsePersistedComposerDrafts(persisted);
        if (!drafts) return current;
        return { ...current, drafts };
      },
    },
  ),
);

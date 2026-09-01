import { nanoid } from "nanoid";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { fileToInputImage } from "@/plugins/builtin/chat/composer/public/input";
import { countLines } from "@/plugins/builtin/chat/composer/public/largePaste";
import { t } from "@/lib/i18n";
import { discardOlderVersions } from "@/lib/persistedStore";
import { notifyError } from "@/plugins/sdk";
import type { ComposerImage } from "../domain/draft";
import type { ComposerModelPreference } from "../application/ports/state";
import { Composer } from "../domain/composer";
import { parsePersistedComposer, persistedComposerDrafts } from "./persistedDrafts";

const STORAGE_KEY = "flame.composer";

interface ComposerState {
  composer: Composer;
  /** GLOBAL, not per session: an explicit model pick is not per-conversation work. */
  modelPreference: ComposerModelPreference;
}

interface ComposerActions {
  setValue: (value: string) => void;
  setModel: (preference: ComposerModelPreference) => void;
  clear: () => void;
  addImages: (images: readonly Omit<ComposerImage, "id">[]) => void;
  /** Fire-and-forget and per-file tolerant. Dropped entirely if the composer is cleared or
   *  the active session switches mid-decode, so a late image cannot leak into the next
   *  message or another conversation's draft. */
  addImageFiles: (files: File[]) => void;
  removeImage: (id: string) => void;
  /** Keeps a large blob out of the textarea; re-inlined into the message on send. */
  addPaste: (text: string) => void;
  removePaste: (id: string) => void;
  loadSession: (sessionId: string) => void;
  pruneDrafts: (liveSessionIds: Set<string>) => void;
  pushHistory: (text: string) => void;
  /** False when there is no history to recall, so the key falls through to cursor movement. */
  historyPrev: () => boolean;
  /** False when not currently navigating history. */
  historyNext: () => boolean;
}

export const useComposerStore = create<ComposerState & ComposerActions>()(
  persist(
    (set, get) => {
      // Replaced on every clear(). `addImageFiles` captures it when its decode starts and
      // drops the result if it advanced, so an image still decoding at submit time is
      // discarded rather than leaking into the NEXT message.
      let stagingLease: object = {};
      const edit = (change: Parameters<Composer["edit"]>[0]) =>
        set((s) => ({ composer: s.composer.edit(change) }));

      return {
        composer: Composer.empty(),
        modelPreference: { kind: "session" },

        setValue: (value) => edit((draft) => draft.withValue(value)),
        setModel: (modelPreference) => set({ modelPreference }),
        clear: () => {
          stagingLease = {};
          set((s) => ({ composer: s.composer.clear() }));
        },
        addImages: (images) =>
          edit((draft) =>
            draft.withImages([
              ...draft.images,
              ...images.map((image) => ({ id: nanoid(), ...image })),
            ]),
          ),
        addImageFiles: (files) => {
          const lease = stagingLease;
          const sessionId = get().composer.activeSessionId;
          // `allSettled`, not `all`: one unreadable file must not discard the batch, and
          // the chain must never reject (there is no global rejection handler).
          void Promise.allSettled(files.map(fileToInputImage)).then((results) => {
            // A stale lease or a switched session must drop the result entirely.
            if (lease !== stagingLease || get().composer.activeSessionId !== sessionId) return;
            const ok = results.flatMap((r) => (r.status === "fulfilled" ? [r.value] : []));
            if (ok.length > 0) get().addImages(ok);
            const failed = results.length - ok.length;
            if (failed > 0) {
              notifyError(
                failed > 1
                  ? t("composer.error.readImages", { count: failed })
                  : t("composer.error.readImage"),
                { source: "composer" },
              );
            }
          });
        },
        removeImage: (id) =>
          edit((draft) => draft.withImages(draft.images.filter((image) => image.id !== id))),
        addPaste: (text) =>
          edit((draft) =>
            draft.withPastes([...draft.pastes, { id: nanoid(), text, lines: countLines(text) }]),
          ),
        removePaste: (id) =>
          edit((draft) => draft.withPastes(draft.pastes.filter((paste) => paste.id !== id))),
        loadSession: (sessionId) => set((s) => ({ composer: s.composer.activate(sessionId) })),
        pruneDrafts: (liveSessionIds) =>
          set((s) => ({ composer: s.composer.prune(liveSessionIds) })),
        pushHistory: (text) => set((s) => ({ composer: s.composer.record(text) })),
        historyPrev: () => {
          const recalled = get().composer.recallOlder();
          if (!recalled) return false;
          set({ composer: recalled });
          return true;
        },
        historyNext: () => {
          const recalled = get().composer.recallNewer();
          if (!recalled) return false;
          set({ composer: recalled });
          return true;
        },
      };
    },
    {
      name: STORAGE_KEY,
      storage: createJSONStorage(() => localStorage),
      version: 2,
      migrate: discardOlderVersions,
      // Text-only: images are transient and the model fallback comes from the active Session.
      partialize: (s) => ({ drafts: persistedComposerDrafts(s.composer) }),
      merge: (persisted, current) => {
        const composer = parsePersistedComposer(persisted);
        return composer ? { ...current, composer } : current;
      },
    },
  ),
);

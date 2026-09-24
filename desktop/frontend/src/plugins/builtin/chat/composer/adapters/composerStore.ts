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
  modelPreference: ComposerModelPreference;
}

interface ComposerActions {
  setValue: (value: string) => void;
  setModel: (preference: ComposerModelPreference) => void;
  clear: () => void;
  addImages: (images: readonly Omit<ComposerImage, "id">[]) => void;
  addImageFiles: (files: File[]) => void;
  removeImage: (id: string) => void;
  addPaste: (text: string) => void;
  removePaste: (id: string) => void;
  editPaste: (id: string, text: string) => void;
  restorePaste: (id: string) => void;
  loadSession: (sessionId: string) => void;
  pruneDrafts: (liveSessionIds: Set<string>) => void;
  pushHistory: (text: string) => void;
  historyPrev: () => boolean;
  historyNext: () => boolean;
}

export const useComposerStore = create<ComposerState & ComposerActions>()(
  persist(
    (set, get) => {
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
          void Promise.allSettled(files.map(fileToInputImage)).then((results) => {
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
        editPaste: (id, text) => edit((draft) => draft.editPaste(id, text)),
        restorePaste: (id) => edit((draft) => draft.restorePaste(id)),
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
      partialize: (s) => ({ drafts: persistedComposerDrafts(s.composer) }),
      merge: (persisted, current) => {
        const composer = parsePersistedComposer(persisted);
        return composer ? { ...current, composer } : current;
      },
    },
  ),
);

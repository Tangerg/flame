import { nanoid } from "nanoid";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { fileToInputImage } from "@/features/composer/public/input";
import { countLines } from "@/features/composer/public/largePaste";
import { ScopedPersistence } from "@/lib/persistedStore";
import {
  type ComposerDraftInput,
  type ComposerImage,
  type ImageReadResult,
} from "@/features/composer/domain/draft";
import {
  type ComposerDraftReplacement,
  type ComposerModelPreference,
  type ComposerSubmission,
} from "@/features/composer/application/state";
import { Composer, ComposerDraft } from "@/features/composer/domain/composer";
import {
  parsePersistedComposer,
  persistedComposerDrafts,
} from "@/features/composer/adapters/persistedDrafts";
const STORAGE_KEY = "flame.composer";

interface ComposerState {
  composer: Composer;
  modelPreference: ComposerModelPreference;
}

interface ComposerActions {
  setValue: (value: string) => void;
  setModel: (preference: ComposerModelPreference) => void;
  clear: () => void;
  captureSubmission: () => ComposerSubmission;
  replaceDraft: (input: ComposerDraftInput) => void;
  captureDraftReplacement: () => ComposerDraftReplacement;
  addImages: (images: readonly Omit<ComposerImage, "id">[]) => void;
  addImageFiles: (files: File[]) => void;
  removeImage: (id: string) => void;
  addPaste: (text: string) => void;
  removePaste: (id: string) => void;
  editPaste: (id: string, text: string) => boolean;
  restorePaste: (id: string, text: string) => void;
  loadSession: (sessionId: string) => void;
  discardSession: (sessionId: string) => void;
  historyPrev: () => boolean;
  historyNext: () => boolean;
}

export function createComposerStore() {
  const persistence = new ScopedPersistence<ComposerState>(STORAGE_KEY);

  let storageGeneration: object = {};

  function emptyComposerState(): ComposerState {
    return { composer: Composer.empty(), modelPreference: { kind: "session" } };
  }

  const useComposerStore = create<ComposerState & ComposerActions>()(
    persist(
      (set, get) => {
        const edit = (change: Parameters<Composer["edit"]>[0]) =>
          set((s) => ({ composer: s.composer.edit(change) }));

        return {
          ...emptyComposerState(),

          setValue: (value) => edit((draft) => draft.withValue(value)),
          setModel: (modelPreference) => set({ modelPreference }),
          clear: () => {
            set((s) => ({ composer: s.composer.clear() }));
          },
          captureSubmission: () => {
            const generation = storageGeneration;
            const { activeSessionId: sessionId, draft } = get().composer;
            return {
              value: draft.value,
              images: draft.images,
              attachmentsReady: draft.attachmentsReady,
              pastes: draft.pastes,
              consume: (historyText) => {
                if (generation !== storageGeneration) return false;
                const current = get().composer;
                const matches =
                  draft.attachmentsReady && current.activate(sessionId).draft === draft;
                set({ composer: current.consumeDraft(sessionId, draft, historyText) });
                return matches && current.activeSessionId === sessionId;
              },
            };
          },
          replaceDraft: (input) => {
            const images = (input.images ?? []).map((image) => ({ ...image, id: nanoid() }));
            edit(() => ComposerDraft.restoreText(input.text).withImages(images));
          },
          captureDraftReplacement: () => {
            const generation = storageGeneration;
            const { activeSessionId: sessionId, draft } = get().composer;
            return (input) => {
              if (generation !== storageGeneration) return false;
              const current = get();
              if (
                current.composer.activeSessionId !== sessionId ||
                current.composer.draft !== draft
              ) {
                return false;
              }
              current.replaceDraft(input);
              return true;
            };
          },
          addImages: (images) =>
            edit((draft) =>
              draft.appendImages(images.map((image) => ({ ...image, id: nanoid() }))),
            ),
          addImageFiles: (files) => {
            if (files.length === 0) return;
            const sessionId = get().composer.activeSessionId;
            const generation = storageGeneration;
            const selected = files.map((file) => ({ id: nanoid(), name: file.name }));
            edit((draft) => draft.beginImageRead(selected));
            const settle = (id: string, result: ImageReadResult) => {
              if (generation !== storageGeneration) return;
              set((state) => ({ composer: state.composer.settleImageRead(sessionId, id, result) }));
            };
            files.forEach((file, index) => {
              const { id } = selected[index]!;
              void fileToInputImage(file).then(
                (image) => settle(id, { status: "ready", image }),
                () => settle(id, { status: "failed" }),
              );
            });
          },
          removeImage: (id) => edit((draft) => draft.removeImage(id)),
          addPaste: (text) =>
            edit((draft) =>
              draft.withPastes([...draft.pastes, { id: nanoid(), text, lines: countLines(text) }]),
            ),
          removePaste: (id) =>
            edit((draft) => draft.withPastes(draft.pastes.filter((paste) => paste.id !== id))),
          editPaste: (id, text) => {
            edit((draft) => draft.editPaste(id, text));
            return get().composer.draft.pastes.some((paste) => paste.id === id);
          },
          restorePaste: (id, text) => edit((draft) => draft.restorePaste(id, text)),
          loadSession: (sessionId) => set((s) => ({ composer: s.composer.activate(sessionId) })),
          discardSession: (sessionId) => {
            set((s) => ({ composer: s.composer.discardSession(sessionId) }));
          },
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
        storage: createJSONStorage(() => persistence.storage),
        skipHydration: true,
        onRehydrateStorage: () => (_state, error) => {
          if (error) useComposerStore.setState(emptyComposerState());
        },
        partialize: (s) => ({ drafts: persistedComposerDrafts(s.composer) }),
        merge: (persisted, current) => {
          const composer = parsePersistedComposer(persisted);
          return { ...current, ...emptyComposerState(), ...(composer ? { composer } : {}) };
        },
      },
    ),
  );

  function activateComposerStorage(endpoint: string): boolean {
    if (persistence.isActive(endpoint)) return false;
    storageGeneration = {};
    return persistence.activate(endpoint, useComposerStore);
  }

  return {
    store: useComposerStore,
    activate: activateComposerStorage,
    dispose() {
      storageGeneration = {};
    },
  };
}

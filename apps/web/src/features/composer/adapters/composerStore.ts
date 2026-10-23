import { nanoid } from "nanoid";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { fileToInputImage } from "@/features/composer/public/input";
import { countLines } from "@/features/composer/public/largePaste";
import { t } from "@/lib/i18n/index";
import { ScopedPersistence } from "@/lib/persistedStore";
import { notifyError } from "@/features/notifications/public/notifications";
import { type ComposerDraftInput, type ComposerImage } from "@/features/composer/domain/draft";
import {
  type ComposerDraftReplacement,
  type ComposerModelPreference,
  type ComposerSubmission,
} from "@/features/composer/application/ports/state";
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

const persistence = new ScopedPersistence<ComposerState>(STORAGE_KEY);

const imageStaging = new Map<string, { latestIntake: object }>();

let storageGeneration: object = {};

function emptyComposerState(): ComposerState {
  return { composer: Composer.empty(), modelPreference: { kind: "session" } };
}

export const useComposerStore = create<ComposerState & ComposerActions>()(
  persist(
    (set, get) => {
      const edit = (change: Parameters<Composer["edit"]>[0]) =>
        set((s) => ({ composer: s.composer.edit(change) }));

      return {
        ...emptyComposerState(),

        setValue: (value) => edit((draft) => draft.withValue(value)),
        setModel: (modelPreference) => set({ modelPreference }),
        clear: () => {
          imageStaging.delete(get().composer.activeSessionId);
          set((s) => ({ composer: s.composer.clear() }));
        },
        captureSubmission: () => {
          const generation = storageGeneration;
          const { activeSessionId: sessionId, draft } = get().composer;
          return {
            value: draft.value,
            images: draft.images,
            pastes: draft.pastes,
            consume: (historyText) => {
              if (generation !== storageGeneration) return false;
              const current = get().composer;
              const matches = current.activate(sessionId).draft === draft;
              set({ composer: current.consumeDraft(sessionId, draft, historyText) });
              if (matches) imageStaging.delete(sessionId);
              return matches && current.activeSessionId === sessionId;
            },
          };
        },
        replaceDraft: (input) => {
          imageStaging.delete(get().composer.activeSessionId);
          const images = (input.images ?? []).map((image) => ({ id: nanoid(), ...image }));
          edit(() => ComposerDraft.restoreText(input.text).withImages(images));
        },
        captureDraftReplacement: () => {
          const generation = storageGeneration;
          const { activeSessionId: sessionId, draft } = get().composer;
          const imageIntake = imageStaging.get(sessionId)?.latestIntake;
          return (input) => {
            if (generation !== storageGeneration) return false;
            const current = get();
            if (
              current.composer.activeSessionId !== sessionId ||
              current.composer.draft !== draft ||
              imageStaging.get(sessionId)?.latestIntake !== imageIntake
            ) {
              return false;
            }
            current.replaceDraft(input);
            return true;
          };
        },
        addImages: (images) =>
          edit((draft) =>
            draft.withImages([
              ...draft.images,
              ...images.map((image) => ({ id: nanoid(), ...image })),
            ]),
          ),
        addImageFiles: (files) => {
          if (files.length === 0) return;
          const sessionId = get().composer.activeSessionId;
          const intake = {};
          const lease = imageStaging.get(sessionId) ?? { latestIntake: intake };
          lease.latestIntake = intake;
          imageStaging.set(sessionId, lease);
          void Promise.allSettled(files.map(fileToInputImage)).then((results) => {
            if (
              lease !== imageStaging.get(sessionId) ||
              get().composer.activeSessionId !== sessionId
            )
              return;
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
        editPaste: (id, text) => {
          edit((draft) => draft.editPaste(id, text));
          return get().composer.draft.pastes.some((paste) => paste.id === id);
        },
        restorePaste: (id, text) => edit((draft) => draft.restorePaste(id, text)),
        loadSession: (sessionId) => set((s) => ({ composer: s.composer.activate(sessionId) })),
        discardSession: (sessionId) => {
          imageStaging.delete(sessionId);
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

export function activateComposerStorage(endpoint: string): boolean {
  if (persistence.isActive(endpoint)) return false;
  imageStaging.clear();
  storageGeneration = {};
  return persistence.activate(endpoint, useComposerStore);
}

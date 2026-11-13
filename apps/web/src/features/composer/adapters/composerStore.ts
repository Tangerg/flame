import { create } from "zustand";
import { fileToInputImage } from "@/features/composer/public/input";
import { countLines } from "@/features/composer/public/largePaste";
import {
  type ComposerDraftInput,
  type ComposerImage,
  type ImageReadResult,
} from "@/features/composer/domain/draft";
import {
  type ComposerDraftReplacement,
  type ComposerModelPreference,
  type ComposerPersistenceState,
  type ComposerSubmission,
} from "@/features/composer/application/state";
import { Composer, ComposerDraft } from "@/features/composer/domain/composer";
import {
  decodePersistedComposer,
  encodePersistedComposer,
  persistedComposerDrafts,
} from "@/features/composer/adapters/persistedDrafts";
const STORAGE_KEY = "flame.composer";
const SAVED: ComposerPersistenceState = { status: "saved" };

interface ComposerState {
  composer: Composer;
  modelPreference: ComposerModelPreference;
  persistenceState: ComposerPersistenceState;
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
  const snapshots = new Map<string, ComposerState>();
  let endpoint: string | null = null;
  let storageGeneration: object = {};

  function emptyComposerState(): ComposerState {
    return {
      composer: Composer.empty(),
      modelPreference: { kind: "session" },
      persistenceState: SAVED,
    };
  }

  function read(target: string): ComposerState {
    try {
      const raw = localStorage.getItem(`${STORAGE_KEY}:${encodeURIComponent(target)}`);
      return {
        ...emptyComposerState(),
        ...(raw === null ? {} : { composer: decodePersistedComposer(raw) }),
      };
    } catch (cause) {
      return {
        ...emptyComposerState(),
        persistenceState: { status: "failed", operation: "read", cause },
      };
    }
  }

  function confirm(
    state: ComposerState,
    authored: boolean,
    textChanged: boolean,
  ): ComposerPersistenceState {
    if (endpoint === null) return state.persistenceState;
    if (state.persistenceState.status === "recovery-required")
      return authored ? { ...state.persistenceState } : state.persistenceState;
    if (state.persistenceState.status === "failed" && state.persistenceState.operation === "read") {
      return authored
        ? { status: "recovery-required", cause: state.persistenceState.cause }
        : state.persistenceState;
    }
    if (!textChanged) return state.persistenceState;
    try {
      localStorage.setItem(
        `${STORAGE_KEY}:${encodeURIComponent(endpoint)}`,
        encodePersistedComposer(state.composer),
      );
      return SAVED;
    } catch (cause) {
      return { status: "failed", operation: "save", cause };
    }
  }

  const useComposerStore = create<ComposerState & ComposerActions>()((set, get) => {
    const publish = (
      change: Partial<ComposerState> | ((state: ComposerState) => Partial<ComposerState>),
      sessionId: string | false = get().composer.activeSessionId,
    ) =>
      set((state) => {
        const next = { ...state, ...(typeof change === "function" ? change(state) : change) };
        let authored = false;
        let textChanged = false;
        if (sessionId !== false) {
          const before = state.composer.activate(sessionId).draft;
          const after = next.composer.activate(sessionId).draft;
          textChanged =
            before.value !== after.value ||
            before.pastes.length !== after.pastes.length ||
            before.pastes.some(
              (paste, index) =>
                paste.id !== after.pastes[index]?.id || paste.text !== after.pastes[index]?.text,
            );
          authored =
            textChanged ||
            before.imageAttachments.length !== after.imageAttachments.length ||
            before.imageAttachments.some((image, index) => image !== after.imageAttachments[index]);
        }
        return { ...next, persistenceState: confirm(next, authored, textChanged) };
      });
    const edit = (change: Parameters<Composer["edit"]>[0]) =>
      publish((s) => ({ composer: s.composer.edit(change) }));

    return {
      ...emptyComposerState(),

      setValue: (value) => edit((draft) => draft.withValue(value)),
      setModel: (modelPreference) => publish({ modelPreference }, false),
      clear: () => {
        publish((s) => ({ composer: s.composer.clear() }));
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
            const matches = draft.attachmentsReady && current.activate(sessionId).draft === draft;
            publish({ composer: current.consumeDraft(sessionId, draft, historyText) }, sessionId);
            return matches && current.activeSessionId === sessionId;
          },
        };
      },
      replaceDraft: (input) => {
        const images = (input.images ?? []).map((image) => ({
          ...image,
          id: crypto.randomUUID(),
        }));
        edit(() => ComposerDraft.restoreText(input.text).withImages(images));
      },
      captureDraftReplacement: () => {
        const generation = storageGeneration;
        const { activeSessionId: sessionId, draft } = get().composer;
        return (input) => {
          if (generation !== storageGeneration) return false;
          const current = get();
          if (current.composer.activeSessionId !== sessionId || current.composer.draft !== draft) {
            return false;
          }
          current.replaceDraft(input);
          return true;
        };
      },
      addImages: (images) =>
        edit((draft) =>
          draft.appendImages(images.map((image) => ({ ...image, id: crypto.randomUUID() }))),
        ),
      addImageFiles: (files) => {
        if (files.length === 0) return;
        const sessionId = get().composer.activeSessionId;
        const generation = storageGeneration;
        const selected = files.map((file) => ({ id: crypto.randomUUID(), name: file.name }));
        edit((draft) => draft.beginImageRead(selected));
        const settle = (id: string, result: ImageReadResult) => {
          if (generation !== storageGeneration) return;
          publish(
            (state) => ({ composer: state.composer.settleImageRead(sessionId, id, result) }),
            sessionId,
          );
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
          draft.withPastes([
            ...draft.pastes,
            { id: crypto.randomUUID(), text, lines: countLines(text) },
          ]),
        ),
      removePaste: (id) =>
        edit((draft) => draft.withPastes(draft.pastes.filter((paste) => paste.id !== id))),
      editPaste: (id, text) => {
        edit((draft) => draft.editPaste(id, text));
        return get().composer.draft.pastes.some((paste) => paste.id === id);
      },
      restorePaste: (id, text) => edit((draft) => draft.restorePaste(id, text)),
      loadSession: (sessionId) =>
        publish((s) => ({ composer: s.composer.activate(sessionId) }), false),
      discardSession: (sessionId) => {
        publish((s) => ({ composer: s.composer.discardSession(sessionId) }), sessionId);
      },
      historyPrev: () => {
        const recalled = get().composer.recallOlder();
        if (!recalled) return false;
        publish({ composer: recalled });
        return true;
      },
      historyNext: () => {
        const recalled = get().composer.recallNewer();
        if (!recalled) return false;
        publish({ composer: recalled });
        return true;
      },
    };
  });

  function activateComposerStorage(target: string): boolean {
    if (endpoint === target) return false;
    if (endpoint !== null) {
      const { composer, modelPreference, persistenceState } = useComposerStore.getState();
      snapshots.set(endpoint, { composer, modelPreference, persistenceState });
    }
    storageGeneration = {};
    endpoint = target;
    const retained = snapshots.get(target);
    snapshots.delete(target);
    useComposerStore.setState(retained ?? read(target));
    return true;
  }

  function retryPersistence(): void {
    if (endpoint === null) return;
    const current = useComposerStore.getState();
    if (current.persistenceState.status !== "failed") return;
    if (current.persistenceState.operation === "save") {
      useComposerStore.setState({ persistenceState: confirm(current, false, true) });
      return;
    }
    const restored = read(endpoint);
    if (restored.persistenceState.status === "saved") {
      useComposerStore.setState({
        composer: restored.composer.activate(current.composer.activeSessionId),
        persistenceState: SAVED,
      });
    } else {
      useComposerStore.setState({ persistenceState: restored.persistenceState });
    }
  }

  function unconfirmedTargets() {
    const targets = [...snapshots].map(([target, state]) => ({
      active: target === endpoint,
      state,
    }));
    if (endpoint !== null) targets.push({ active: true, state: useComposerStore.getState() });
    return targets.filter(({ state }) => state.persistenceState.status !== "saved");
  }

  return {
    store: useComposerStore,
    activate: activateComposerStorage,
    retryPersistence,
    hasUnconfirmedDrafts: () => unconfirmedTargets().length > 0,
    subscribePersistence: (listener: () => void) =>
      useComposerStore.subscribe((state, previous) => {
        if (state.persistenceState !== previous.persistenceState) listener();
      }),
    getRecoveryDrafts: () =>
      JSON.stringify(
        {
          targets: unconfirmedTargets().map(({ active, state }) => ({
            active,
            drafts: persistedComposerDrafts(state.composer),
          })),
        },
        null,
        2,
      ),
    dispose() {
      storageGeneration = {};
    },
  };
}

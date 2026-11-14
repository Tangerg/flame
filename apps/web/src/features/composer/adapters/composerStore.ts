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
  type ComposerLegacyDraftState,
  type ComposerSubmission,
} from "@/features/composer/application/state";
import { Composer, ComposerDraft } from "@/features/composer/domain/composer";
import {
  decodeLegacyComposer,
  persistedComposerDrafts,
} from "@/features/composer/adapters/persistedDrafts";
import { DraftDatabase } from "@/features/composer/adapters/draftDatabase";
const SAVED: ComposerPersistenceState = { status: "saved" };
const NONE: ComposerLegacyDraftState = { status: "none" };
const legacyKey = (endpoint: string) => `flame.composer:${encodeURIComponent(endpoint)}`;

interface ComposerState {
  composer: Composer;
  modelPreference: ComposerModelPreference;
  persistenceState: ComposerPersistenceState;
  legacyDraftState: ComposerLegacyDraftState;
}
interface ComposerActions {
  setValue(value: string): void;
  setModel(preference: ComposerModelPreference): void;
  clear(): void;
  captureSubmission(): ComposerSubmission;
  replaceDraft(input: ComposerDraftInput): void;
  captureDraftReplacement(): ComposerDraftReplacement;
  addImages(images: readonly Omit<ComposerImage, "id">[]): void;
  addImageFiles(files: File[]): void;
  removeImage(id: string): void;
  addPaste(text: string): void;
  removePaste(id: string): void;
  editPaste(id: string, text: string): boolean;
  restorePaste(id: string, text: string): void;
  loadSession(sessionId: string): void;
  discardSession(sessionId: string): void;
  historyPrev(): boolean;
  historyNext(): boolean;
}
interface Target {
  endpoint: string | null;
  state: ComposerState;
  hydrated: boolean;
  initialized: boolean;
  reading: boolean;
  writing: boolean;
  touched: Set<string>;
  textTouched: Set<string>;
  dirty: Map<string, ComposerDraft>;
}
interface ImageRead {
  target: Target;
  sessionId: string;
  id: string;
  file: File;
  controller: AbortController;
}

export function createComposerStore() {
  const database = new DraftDatabase();
  const targets = new Map<string, Target>();
  let storageGeneration: object = {};
  let disposed = false;
  const emptyState = (): ComposerState => ({
    composer: Composer.empty(),
    modelPreference: { kind: "session" },
    persistenceState: SAVED,
    legacyDraftState: NONE,
  });
  const target = (endpoint: string | null): Target => ({
    endpoint,
    state: emptyState(),
    hydrated: endpoint === null,
    initialized: false,
    reading: false,
    writing: false,
    touched: new Set(),
    textTouched: new Set(),
    dirty: new Map(),
  });
  let current = target(null);
  const reads: ImageRead[] = [];
  let activeRead: ImageRead | null = null;
  const persistenceListeners = new Set<() => void>();

  function present(owner: Target) {
    if (disposed) return;
    if (owner === current) useComposerStore.setState(owner.state);
    for (const listener of persistenceListeners) listener();
  }
  function persistence(owner: Target, state: ComposerPersistenceState) {
    owner.state = { ...owner.state, persistenceState: state };
    present(owner);
  }
  function legacy(owner: Target, state: ComposerLegacyDraftState) {
    owner.state = { ...owner.state, legacyDraftState: state };
    present(owner);
  }
  async function flush(owner: Target) {
    if (
      disposed ||
      owner.endpoint === null ||
      !owner.hydrated ||
      owner.writing ||
      !owner.dirty.size ||
      owner.state.persistenceState.status === "failed" ||
      owner.state.persistenceState.status === "recovery-required"
    )
      return;
    owner.writing = true;
    const batch = new Map(owner.dirty);
    try {
      await database.write(owner.endpoint, batch);
      owner.initialized = true;
      for (const [session, draft] of batch)
        if (owner.dirty.get(session) === draft) owner.dirty.delete(session);
      persistence(owner, owner.dirty.size ? { status: "pending" } : SAVED);
    } catch (cause) {
      persistence(owner, { status: "failed", operation: "save", cause });
    } finally {
      owner.writing = false;
    }
    if (!disposed && owner.state.persistenceState.status === "pending") void flush(owner);
  }
  async function hydrate(owner: Target) {
    if (owner.endpoint === null || owner.reading) return;
    owner.reading = true;
    persistence(owner, { status: "loading" });
    try {
      const loaded = await database.read(owner.endpoint);
      if (disposed) return;
      owner.hydrated = true;
      owner.initialized = loaded.initialized;
      owner.state = {
        ...owner.state,
        composer: owner.state.composer.hydrateDrafts(loaded.drafts, owner.textTouched),
      };
      try {
        const raw = localStorage.getItem(legacyKey(owner.endpoint));
        owner.state = {
          ...owner.state,
          legacyDraftState:
            raw !== null && !loaded.imported
              ? { status: loaded.initialized || owner.touched.size > 0 ? "blocked" : "available" }
              : NONE,
        };
      } catch (cause) {
        owner.state = {
          ...owner.state,
          legacyDraftState: {
            status: "failed",
            cause,
            canImport: !loaded.initialized && !owner.touched.size,
          },
        };
      }
      persistence(owner, owner.dirty.size ? { status: "pending" } : SAVED);
      void flush(owner);
    } catch (cause) {
      try {
        if (localStorage.getItem(legacyKey(owner.endpoint)) !== null) {
          owner.state = {
            ...owner.state,
            legacyDraftState: { status: "failed", cause, canImport: false },
          };
        }
      } catch (backupCause) {
        owner.state = {
          ...owner.state,
          legacyDraftState: { status: "failed", cause: backupCause, canImport: false },
        };
      }
      persistence(
        owner,
        owner.touched.size
          ? { status: "recovery-required", cause }
          : { status: "failed", operation: "read", cause },
      );
    } finally {
      owner.reading = false;
    }
  }
  function settle(read: ImageRead, result: ImageReadResult) {
    if (disposed || read.controller.signal.aborted) return;
    const owner = read.target;
    owner.state = {
      ...owner.state,
      composer: owner.state.composer.settleImageRead(read.sessionId, read.id, result),
    };
    present(owner);
  }
  function pumpReads() {
    if (disposed || activeRead) return;
    const read = reads.shift();
    if (!read) return;
    activeRead = read;
    void fileToInputImage(read.file, read.controller.signal)
      .then(
        (image) => settle(read, { status: "ready", image }),
        () => settle(read, { status: "failed" }),
      )
      .finally(() => {
        if (activeRead === read) activeRead = null;
        pumpReads();
      });
  }
  function cancelReads(owner: Target, sessionId?: string, id?: string) {
    const matches = (read: ImageRead) =>
      read.target === owner &&
      (sessionId === undefined || read.sessionId === sessionId) &&
      (id === undefined || read.id === id);
    const cancelled = [
      ...reads.filter(matches),
      ...(activeRead && matches(activeRead) ? [activeRead] : []),
    ];
    for (let i = reads.length - 1; i >= 0; i--) if (matches(reads[i]!)) reads.splice(i, 1);
    for (const read of cancelled) {
      // Keep a removable failure when this endpoint is retained; it must never remain pending forever.
      owner.state = {
        ...owner.state,
        composer: owner.state.composer.settleImageRead(read.sessionId, read.id, {
          status: "failed",
        }),
      };
      read.controller.abort();
    }
    if (cancelled.length) present(owner);
  }

  const useComposerStore = create<ComposerState & ComposerActions>()((_set, get) => {
    const publish = (
      change: Partial<ComposerState> | ((state: ComposerState) => Partial<ComposerState>),
      sessionId: string | false = current.state.composer.activeSessionId,
      forceText = false,
    ) => {
      const owner = current;
      const beforeState = owner.state;
      const next = {
        ...beforeState,
        ...(typeof change === "function" ? change(beforeState) : change),
      };
      if (sessionId !== false) {
        const before = beforeState.composer.activate(sessionId).draft;
        const after = next.composer.activate(sessionId).draft;
        const textChanged = forceText || !before.hasSameText(after);
        const authored = textChanged || !before.hasSameAttachments(after);
        if (authored) {
          owner.touched.add(sessionId);
          if (next.legacyDraftState.status === "available")
            next.legacyDraftState = { status: "blocked" };
          else if (next.legacyDraftState.status === "failed" && next.legacyDraftState.canImport)
            next.legacyDraftState = { ...next.legacyDraftState, canImport: false };
          if (
            beforeState.persistenceState.status === "failed" &&
            beforeState.persistenceState.operation === "read"
          )
            next.persistenceState = {
              status: "recovery-required",
              cause: beforeState.persistenceState.cause,
            };
          else if (beforeState.persistenceState.status === "recovery-required")
            next.persistenceState = { ...beforeState.persistenceState };
          else if (beforeState.persistenceState.status === "failed")
            next.persistenceState = { ...beforeState.persistenceState };
        }
        if (textChanged && owner.endpoint !== null) {
          owner.textTouched.add(sessionId);
          owner.dirty.set(sessionId, after);
          if (
            owner.hydrated &&
            next.persistenceState.status !== "failed" &&
            next.persistenceState.status !== "recovery-required"
          )
            next.persistenceState = { status: "pending" };
        }
      }
      owner.state = next;
      present(owner);
      void flush(owner);
    };
    const edit = (change: Parameters<Composer["edit"]>[0], forceText = false) =>
      publish(
        (state) => ({ composer: state.composer.edit(change) }),
        current.state.composer.activeSessionId,
        forceText,
      );
    return {
      ...current.state,
      setValue: (value) => edit((draft) => draft.withValue(value)),
      setModel: (modelPreference) => publish({ modelPreference }, false),
      clear: () => {
        cancelReads(current, current.state.composer.activeSessionId);
        publish(
          (state) => ({ composer: state.composer.clear() }),
          current.state.composer.activeSessionId,
          true,
        );
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
            publish(
              { composer: current.consumeDraft(sessionId, draft, historyText) },
              sessionId,
              matches,
            );
            return matches && current.activeSessionId === sessionId;
          },
        };
      },
      replaceDraft: (input) => {
        cancelReads(current, current.state.composer.activeSessionId);
        const images = (input.images ?? []).map((image) => ({
          ...image,
          id: crypto.randomUUID(),
        }));
        edit(() => ComposerDraft.restoreText(input.text).withImages(images), true);
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
        if (!files.length || disposed) return;
        const owner = current;
        const sessionId = owner.state.composer.activeSessionId;
        const selected = files.map((file) => ({ id: crypto.randomUUID(), name: file.name }));
        edit((draft) => draft.beginImageRead(selected));
        files.forEach((file, index) =>
          reads.push({
            target: owner,
            sessionId,
            file,
            id: selected[index]!.id,
            controller: new AbortController(),
          }),
        );
        pumpReads();
      },
      removeImage: (id) => {
        cancelReads(current, current.state.composer.activeSessionId, id);
        edit((draft) => draft.removeImage(id));
      },
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
        cancelReads(current, sessionId);
        publish((s) => ({ composer: s.composer.discardSession(sessionId) }), sessionId, true);
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

  function activate(endpoint: string): boolean {
    if (disposed || current.endpoint === endpoint) return false;
    cancelReads(current);
    storageGeneration = {};
    let retained = targets.get(endpoint);
    if (!retained) {
      retained = target(endpoint);
      targets.set(endpoint, retained);
    }
    current = retained;
    present(current);
    if (!current.hydrated && !current.reading && current.state.persistenceState.status === "saved")
      void hydrate(current);
    return true;
  }
  function retryPersistence() {
    if (disposed || current.state.persistenceState.status !== "failed") return;
    if (current.state.persistenceState.operation === "read") void hydrate(current);
    else {
      persistence(current, { status: "pending", retrying: true });
      void flush(current);
    }
  }
  async function importLegacyDrafts() {
    const owner = current;
    const state = owner.state.legacyDraftState;
    if (
      disposed ||
      owner.endpoint === null ||
      !owner.hydrated ||
      owner.initialized ||
      owner.touched.size ||
      owner.writing ||
      !(state.status === "available" || (state.status === "failed" && state.canImport))
    )
      return;
    legacy(owner, { status: "importing" });
    owner.writing = true;
    try {
      const raw = localStorage.getItem(legacyKey(owner.endpoint));
      if (raw === null) throw new Error("the existing draft backup is no longer available");
      const imported = decodeLegacyComposer(raw);
      const drafts = new Map(
        [...imported.durableDraftTexts()].map(([session]) => [
          session,
          imported.activate(session).draft,
        ]),
      );
      await database.write(
        owner.endpoint,
        drafts,
        () => !disposed && !owner.initialized && owner.touched.size === 0,
      );
      if (disposed) return;
      owner.initialized = true;
      owner.state = {
        ...owner.state,
        composer: owner.state.composer.hydrateDrafts(
          imported.durableDraftTexts(),
          owner.textTouched,
        ),
        legacyDraftState: NONE,
      };
      persistence(owner, owner.dirty.size ? { status: "pending" } : SAVED);
    } catch (cause) {
      legacy(owner, {
        status: "failed",
        cause,
        canImport: !owner.initialized && owner.touched.size === 0,
      });
    } finally {
      owner.writing = false;
    }
    void flush(owner);
  }
  const unconfirmed = () =>
    [...targets.values()].filter(
      (owner) =>
        owner.dirty.size > 0 ||
        owner.state.persistenceState.status === "failed" ||
        owner.state.persistenceState.status === "recovery-required",
    );
  return {
    store: useComposerStore,
    activate,
    retryPersistence,
    importLegacyDrafts: () => {
      void importLegacyDrafts();
    },
    getLegacyDraftBackup: () =>
      current.endpoint === null ? "" : (localStorage.getItem(legacyKey(current.endpoint)) ?? ""),
    hasUnconfirmedDrafts: () => unconfirmed().length > 0,
    subscribePersistence: (listener: () => void) => {
      persistenceListeners.add(listener);
      return () => {
        persistenceListeners.delete(listener);
      };
    },
    getRecoveryDrafts: () =>
      JSON.stringify(
        {
          targets: unconfirmed().map((owner) => ({
            active: owner === current,
            drafts: persistedComposerDrafts(owner.state.composer),
          })),
        },
        null,
        2,
      ),
    dispose() {
      if (disposed) return;
      for (const owner of new Set([current, ...targets.values()])) cancelReads(owner);
      disposed = true;
      storageGeneration = {};
      database.close();
      persistenceListeners.clear();
    },
  };
}

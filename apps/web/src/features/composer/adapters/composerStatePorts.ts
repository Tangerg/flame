import { useMemo, useSyncExternalStore } from "react";
import { type AgentSessions } from "@/features/agent/public/services";
import {
  followRuntimeGeneration,
  type RuntimeStream,
  type RuntimeServerScope,
} from "@/features/connection/public/services";
import { type ComposerStatePort } from "@/features/composer/application/state";
import { createComposerStore } from "@/features/composer/adapters/composerStore";
import { joinDraftParts } from "@/features/composer/domain/draft";
export function createComposer(
  sessions: AgentSessions,
  endpoint: () => string,
  scope: RuntimeServerScope,
  stream: RuntimeStream,
) {
  const owner = createComposerStore();
  const useComposerStore = owner.store;
  const activateComposerStorage = owner.activate;
  let stopSessionSync: (() => void) | null = null;
  let disposed = false;
  let inputScope = {};
  const inputScopeListeners = new Set<() => void>();
  const subscribeInputScope = (listener: () => void) => {
    inputScopeListeners.add(listener);
    return () => {
      inputScopeListeners.delete(listener);
    };
  };
  const advanceInputScope = () => {
    inputScope = {};
    inputScopeListeners.forEach((listener) => listener());
  };
  const stopInputScope = useComposerStore.subscribe((state, previous) => {
    if (state.composer.activeSessionId !== previous.composer.activeSessionId) advanceInputScope();
  });

  let focusTarget: {
    element: HTMLTextAreaElement | null;
    synchronizeSelection?: () => void;
  } | null = null;
  let focusPresentation: { present: (focus: () => void) => void } | null = null;
  const focus = (selectionEnd?: number) => {
    const target = focusTarget;
    if (!target?.element) return;
    const element = target.element;
    element.focus();
    if (selectionEnd !== undefined) element.setSelectionRange(selectionEnd, selectionEnd);
    if (focusTarget === target && element.ownerDocument.activeElement === element)
      target.synchronizeSelection?.();
  };
  const presentFocus = (deliver: () => void) => {
    const target = focusTarget;
    const focusOriginalTarget = () => {
      if (focusTarget === target) deliver();
    };
    if (focusPresentation) focusPresentation.present(focusOriginalTarget);
    else focusOriginalTarget();
  };
  const access: ComposerStatePort = {
    focus,
    captureFocus: () => {
      const target = focusTarget;
      const document = target?.element?.ownerDocument ?? window.document;
      const origin = document.activeElement;
      const source = inputScope;
      return () => {
        if (
          !disposed &&
          inputScope === source &&
          document.activeElement === origin &&
          focusTarget === target
        )
          focus();
      };
    },
    registerFocusTarget: (element, synchronizeSelection) => {
      const target = { element, synchronizeSelection };
      focusTarget = target;
      return () => {
        if (focusTarget === target) focusTarget = null;
      };
    },
    registerFocusPresentation: (present) => {
      const presentation = { present };
      focusPresentation = presentation;
      return () => {
        if (focusPresentation === presentation) focusPresentation = null;
      };
    },
    useText: () => useComposerStore((state) => state.composer.draft.value),
    useSetText: () => useComposerStore((state) => state.setValue),
    usePersistenceState: () => useComposerStore((state) => state.persistenceState),
    retryPersistence: owner.retryPersistence,
    getRecoveryDrafts: owner.getRecoveryDrafts,
    captureSubmission: () => useComposerStore.getState().captureSubmission(),
    replaceDraft: (input) => {
      useComposerStore.getState().replaceDraft(input);
      presentFocus(() => focus(input.text.length));
    },
    beginDraftReplacement: () => {
      const replace = useComposerStore.getState().captureDraftReplacement();
      const target = focusTarget;
      let selectionEnd: number | undefined;
      presentFocus(() => focus(selectionEnd));
      return (input) => {
        if (!replace(input)) return false;
        // A fast rollback can settle before the original presentation request.
        selectionEnd = input.text.length;
        const element = target?.element;
        if (target && element && focusTarget === target && document.activeElement === element) {
          element.setSelectionRange(selectionEnd, selectionEnd);
          target.synchronizeSelection?.();
        }
        return true;
      };
    },
    appendText: (text) => {
      const store = useComposerStore.getState();
      const value = joinDraftParts([store.composer.draft.value.trimEnd(), text]);
      store.setValue(value);
      presentFocus(() => focus(value.length));
    },
    useImageAttachments: () => useComposerStore((state) => state.composer.draft.imageAttachments),
    useAttachmentsReady: () => useComposerStore((state) => state.composer.draft.attachmentsReady),
    usePastes: () => useComposerStore((state) => state.composer.draft.pastes),
    useAddImageFiles: () => useComposerStore((state) => state.addImageFiles),
    useImageIntake: () => {
      const source = useSyncExternalStore(
        subscribeInputScope,
        () => inputScope,
        () => inputScope,
      );
      return useMemo(() => {
        const targetEndpoint = endpoint();
        const session = useComposerStore.getState().composer.activeSessionId;
        const current = () =>
          !disposed &&
          inputScope === source &&
          endpoint() === targetEndpoint &&
          useComposerStore.getState().composer.activeSessionId === session;
        return {
          current,
          addFiles(files: File[]) {
            if (current()) useComposerStore.getState().addImageFiles(files);
          },
        };
      }, [source]);
    },
    useRemoveImage: () => useComposerStore((state) => state.removeImage),
    useAddPaste: () => useComposerStore((state) => state.addPaste),
    useRemovePaste: () => useComposerStore((state) => state.removePaste),
    useEditPaste: () => useComposerStore((state) => state.editPaste),
    useRestorePaste: () => useComposerStore((state) => state.restorePaste),
    recallPreviousHistory: () => useComposerStore.getState().historyPrev(),
    recallNextHistory: () => useComposerStore.getState().historyNext(),
    getModelPreference: () => {
      return useComposerStore.getState().modelPreference;
    },
    useModelPreference: () => useComposerStore((state) => state.modelPreference),
    useSetModelPreference: () => useComposerStore((state) => state.setModel),
  };
  function dispose() {
    if (disposed) return;
    disposed = true;
    advanceInputScope();
    inputScopeListeners.clear();
    focusTarget = null;
    focusPresentation = null;
    stopInputScope();
    stopSessionSync?.();
    stopSessionSync = null;
    owner.dispose();
  }

  function installComposerSessionSync(
    sessions: AgentSessions,
    endpoint: () => string,
    scope: RuntimeServerScope,
  ): () => void {
    stopSessionSync?.();
    const sync = ({ activeSessionId }: ReturnType<AgentSessions["getLifecycleSnapshot"]>) => {
      activateComposerStorage(endpoint());
      const composer = useComposerStore.getState();
      composer.loadSession(activeSessionId);
    };
    const stopGeneration = followRuntimeGeneration(stream, advanceInputScope);
    const stopLifecycle = sessions.subscribeLifecycle(sync);
    const stopDeletion = sessions.subscribeDeleted((sessionId) => {
      activateComposerStorage(endpoint());
      useComposerStore.getState().discardSession(sessionId);
    });
    const stopScope = scope.subscribeReplacement(() => {
      advanceInputScope();
      sync(sessions.getLifecycleSnapshot());
    });
    const stop = () => {
      stopGeneration();
      stopLifecycle();
      stopDeletion();
      stopScope();
    };
    stopSessionSync = stop;

    sync(sessions.getLifecycleSnapshot());
    return () => {
      stop();
      if (stopSessionSync === stop) stopSessionSync = null;
    };
  }

  return {
    access,
    store: useComposerStore,
    activate: activateComposerStorage,
    hasUnconfirmedDrafts: owner.hasUnconfirmedDrafts,
    subscribePersistence: owner.subscribePersistence,
    start() {
      if (disposed) throw new Error("Composer is disposed");
      if (!stopSessionSync) installComposerSessionSync(sessions, endpoint, scope);
    },
    dispose,
  };
}
export type ComposerOwner = ReturnType<typeof createComposer>;

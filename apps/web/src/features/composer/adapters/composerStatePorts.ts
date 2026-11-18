import { useMemo, useSyncExternalStore } from "react";
import { type AgentSessions } from "@/features/agent/public/services";
import {
  followRuntimeGeneration,
  type RuntimeStream,
  type RuntimeServerScope,
} from "@/features/connection/public/services";
import { type ComposerEditor, type ComposerStatePort } from "@/features/composer/application/state";
import { createComposerStore } from "@/features/composer/adapters/composerStore";
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

  let editor: ComposerEditor | null = null;
  let focusTarget: {
    element: HTMLElement | null;
  } | null = null;
  let focusPresentation: { present: (focus: () => void) => void } | null = null;
  const focus = () => {
    const target = focusTarget;
    if (!target?.element) return;
    const element = target.element;
    element.focus();
    if (editor?.element === element) editor.focus();
  };
  const presentFocus = (deliver: () => void) => {
    const target = focusTarget;
    const focusOriginalTarget = () => {
      if (focusTarget === target) deliver();
    };
    if (focusPresentation) focusPresentation.present(focusOriginalTarget);
    else focusOriginalTarget();
  };
  // Draft transitions run once in the domain; a mounted editor then shows their result.
  const presentDraft = () => editor?.restore(useComposerStore.getState().composer.draft.content);
  const withDraftReplacement: ComposerStatePort["beginDraftReplacement"] = (
    requestConfirmation,
    begin,
  ) => {
    const state = useComposerStore.getState();
    const { draft } = state.composer;
    const { current, replace } = state.captureDraftReplacement();
    const apply = () => current() && begin(replace);
    if (draft.imageAttachments.length > 0 || draft.pastes.length > 0) {
      requestConfirmation(apply);
    } else {
      apply();
    }
  };
  const access: ComposerStatePort = {
    useDraftKey: () =>
      useComposerStore((state) => JSON.stringify([endpoint(), state.composer.activeSessionId])),
    initialContent: () => useComposerStore.getState().composer.draft.content,
    registerEditor: (value) => {
      editor = value;
      return () => {
        if (editor === value) editor = null;
      };
    },
    checkpoint: (content) => useComposerStore.getState().checkpoint(content),
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
    captureEditorFocusReturn: (target) => {
      const capturedEditor = editor;
      const capturedTarget = focusTarget;
      const source = inputScope;
      if (capturedEditor?.element !== target || capturedTarget?.element !== target) return null;
      return () =>
        !disposed &&
        inputScope === source &&
        editor === capturedEditor &&
        focusTarget === capturedTarget &&
        capturedEditor.restoreFocusWithSelection();
    },
    registerFocusTarget: (element) => {
      const target = { element };
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
    startFileReference: () => {
      useComposerStore.getState().startFileReference();
      presentDraft();
      return useComposerStore.getState().composer.draft.value;
    },
    startSlashCommand: () => {
      useComposerStore.getState().startSlashCommand();
      presentDraft();
      return useComposerStore.getState().composer.draft.value;
    },
    useDraftLoading: () => useComposerStore((state) => state.persistenceState.status === "loading"),
    usePersistenceState: () => useComposerStore((state) => state.persistenceState),
    retryPersistence: owner.retryPersistence,
    getRecoveryDrafts: owner.getRecoveryDrafts,
    captureSubmission: () => {
      const draft = useComposerStore.getState().captureSubmission();
      const capturedEditor = editor;
      const revision = capturedEditor?.revision();
      return {
        ...draft,
        consume: (historyText) => {
          if (capturedEditor && editor === capturedEditor && capturedEditor.revision() !== revision)
            return false;
          const consumed = draft.consume(historyText);
          if (consumed && editor === capturedEditor) presentDraft();
          return consumed;
        },
      };
    },
    replaceDraft: (input, requestConfirmation) =>
      withDraftReplacement(requestConfirmation, (replace) => {
        if (!replace(input)) return false;
        presentDraft();
        presentFocus(() => focus());
        return true;
      }),
    beginDraftReplacement: (requestConfirmation, begin) =>
      withDraftReplacement(requestConfirmation, (replace) => {
        const target = focusTarget;
        const started = begin((input) => {
          if (!replace(input)) return false;
          presentDraft();
          // A fast rollback can settle before the original presentation request.
          const element = target?.element;
          if (
            element &&
            focusTarget === target &&
            element.ownerDocument.activeElement === element
          ) {
            editor?.focus();
          }
          return true;
        });
        if (started) presentFocus(() => focus());
        return started;
      }),
    captureAppendText: () => {
      const source = inputScope;
      return (text) => {
        if (disposed || source !== inputScope) return false;
        access.appendText(text);
        return true;
      };
    },
    appendText: (text) => {
      useComposerStore.getState().appendText(text);
      presentDraft();
      presentFocus(() => focus());
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
    useRestorePaste: () => (id, text) => {
      useComposerStore.getState().restorePaste(id, text);
      presentDraft();
    },
    recallPreviousHistory: () => {
      const recalled = useComposerStore.getState().historyPrev();
      if (recalled) presentDraft();
      return recalled;
    },
    recallNextHistory: () => {
      const recalled = useComposerStore.getState().historyNext();
      if (recalled) presentDraft();
      return recalled;
    },
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

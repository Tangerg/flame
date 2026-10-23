import { disposeOnHmr } from "@/lib/hmr";
import { type AgentSessions } from "@/features/agent/public/services";
import { type RuntimeServerScope } from "@/features/connection/public/services";
import { configureComposerStatePort } from "@/features/composer/application/ports/state";
import {
  activateComposerStorage,
  useComposerStore,
} from "@/features/composer/adapters/composerStore";
import { joinDraftParts } from "@/features/composer/domain/draft";
let stopSessionSync: (() => void) | null = null;

export function installComposerStatePorts(
  sessions: AgentSessions,
  endpoint: () => string,
  scope: RuntimeServerScope,
): () => void {
  activateComposerStorage(endpoint());
  let focusTarget: { element: HTMLTextAreaElement | null } | null = null;
  let focusPresentation: { present: (focus: () => void) => void } | null = null;
  const focus = (selectionEnd?: number) => {
    const element = focusTarget?.element;
    if (!element) return;
    element.focus();
    if (selectionEnd !== undefined) element.setSelectionRange(selectionEnd, selectionEnd);
  };
  const presentFocus = (deliver: () => void) => {
    const target = focusTarget;
    const focusOriginalTarget = () => {
      if (focusTarget === target) deliver();
    };
    if (focusPresentation) focusPresentation.present(focusOriginalTarget);
    else focusOriginalTarget();
  };
  const disposePort = configureComposerStatePort({
    focus,
    captureFocus: () => {
      const origin = document.activeElement;
      const target = focusTarget;
      return () => {
        if (document.activeElement === origin && focusTarget === target) focus();
      };
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
    useSetText: () => useComposerStore((state) => state.setValue),
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
        if (element && focusTarget === target && document.activeElement === element) {
          element.setSelectionRange(selectionEnd, selectionEnd);
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
    useImages: () => useComposerStore((state) => state.composer.draft.images),
    usePastes: () => useComposerStore((state) => state.composer.draft.pastes),
    useAddImageFiles: () => useComposerStore((state) => state.addImageFiles),
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
  });
  const disposeSessionSync = installComposerSessionSync(sessions, endpoint, scope);
  return () => {
    focusTarget = null;
    focusPresentation = null;
    disposeSessionSync();
    disposePort();
  };
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
  const stopLifecycle = sessions.subscribeLifecycle(sync);
  const stopDeletion = sessions.subscribeDeleted((sessionId) => {
    activateComposerStorage(endpoint());
    useComposerStore.getState().discardSession(sessionId);
  });
  const stopScope = scope.subscribeReplacement(() => sync(sessions.getLifecycleSnapshot()));
  const stop = () => {
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

disposeOnHmr(() => {
  stopSessionSync?.();
  stopSessionSync = null;
});

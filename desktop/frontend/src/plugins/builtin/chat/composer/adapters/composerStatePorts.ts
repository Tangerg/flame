import { disposeOnHmr } from "@/lib/hmr";
import type { AgentSessions } from "@/plugins/builtin/agent/public/services";
import type { RuntimeServerScope } from "@/plugins/builtin/runtime/public/services";
import { focusComposer } from "../application/focus";
import { configureComposerStatePort } from "../application/ports/state";
import { activateComposerStorage, useComposerStore } from "./composerStore";
import { joinDraftParts } from "../domain/draft";

let stopSessionSync: (() => void) | null = null;

export function installComposerStatePorts(
  sessions: AgentSessions,
  endpoint: () => string,
  scope: RuntimeServerScope,
): () => void {
  activateComposerStorage(endpoint());
  const disposePort = configureComposerStatePort({
    useText: () => useComposerStore((state) => state.composer.draft.value),
    useSetText: () => useComposerStore((state) => state.setValue),
    useClearDraft: () => useComposerStore((state) => state.clear),
    getText: () => useComposerStore.getState().composer.draft.value,
    replaceDraft: (input) => {
      const store = useComposerStore.getState();
      store.clear();
      store.setValue(input.text);
      if (input.images?.length) store.addImages(input.images);
      focusComposer(input.text.length);
    },
    appendText: (text) => {
      const store = useComposerStore.getState();
      const value = joinDraftParts([store.composer.draft.value.trimEnd(), text]);
      store.setValue(value);
      focusComposer(value.length);
    },
    useImages: () => useComposerStore((state) => state.composer.draft.images),
    usePastes: () => useComposerStore((state) => state.composer.draft.pastes),
    useAddImageFiles: () => useComposerStore((state) => state.addImageFiles),
    useRemoveImage: () => useComposerStore((state) => state.removeImage),
    useAddPaste: () => useComposerStore((state) => state.addPaste),
    useRemovePaste: () => useComposerStore((state) => state.removePaste),
    useEditPaste: () => useComposerStore((state) => state.editPaste),
    useRestorePaste: () => useComposerStore((state) => state.restorePaste),
    useRecordHistory: () => useComposerStore((state) => state.pushHistory),
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
  const sync = ({
    activeSessionId,
    openSessionIds,
  }: ReturnType<AgentSessions["getLifecycleSnapshot"]>) => {
    activateComposerStorage(endpoint());
    const composer = useComposerStore.getState();
    composer.loadSession(activeSessionId);
    composer.pruneDrafts(new Set(openSessionIds));
  };
  const stopLifecycle = sessions.subscribeLifecycle(sync);
  const stopScope = scope.subscribeReplacement(() => sync(sessions.getLifecycleSnapshot()));
  const stop = () => {
    stopLifecycle();
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

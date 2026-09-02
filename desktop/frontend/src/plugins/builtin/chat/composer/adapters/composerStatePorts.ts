import { disposeOnHmr } from "@/lib/hmr";
import type { AgentSessions } from "@/plugins/builtin/agent/public/services";
import { focusComposer } from "../application/focus";
import { configureComposerStatePort } from "../application/ports/state";
import { useComposerStore } from "./composerStore";

let stopSessionSync: (() => void) | null = null;

export function installComposerStatePorts(sessions: AgentSessions): () => void {
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
    useImages: () => useComposerStore((state) => state.composer.draft.images),
    usePastes: () => useComposerStore((state) => state.composer.draft.pastes),
    useAddImageFiles: () => useComposerStore((state) => state.addImageFiles),
    useRemoveImage: () => useComposerStore((state) => state.removeImage),
    useAddPaste: () => useComposerStore((state) => state.addPaste),
    useRemovePaste: () => useComposerStore((state) => state.removePaste),
    useRecordHistory: () => useComposerStore((state) => state.pushHistory),
    recallPreviousHistory: () => useComposerStore.getState().historyPrev(),
    recallNextHistory: () => useComposerStore.getState().historyNext(),
    getModelPreference: () => {
      return useComposerStore.getState().modelPreference;
    },
    useModelPreference: () => useComposerStore((state) => state.modelPreference),
    useSetModelPreference: () => useComposerStore((state) => state.setModel),
  });
  const disposeSessionSync = installComposerSessionSync(sessions);
  return () => {
    disposeSessionSync();
    disposePort();
  };
}

function installComposerSessionSync(sessions: AgentSessions): () => void {
  stopSessionSync?.();
  const stop = sessions.subscribeLifecycle(({ activeSessionId, openSessionIds }) => {
    const composer = useComposerStore.getState();
    composer.loadSession(activeSessionId);
    composer.pruneDrafts(new Set(openSessionIds));
  });
  stopSessionSync = stop;

  const initial = sessions.getLifecycleSnapshot();
  useComposerStore.getState().loadSession(initial.activeSessionId);
  useComposerStore.getState().pruneDrafts(new Set(initial.openSessionIds));
  return () => {
    stop();
    if (stopSessionSync === stop) stopSessionSync = null;
  };
}

disposeOnHmr(() => {
  stopSessionSync?.();
  stopSessionSync = null;
});

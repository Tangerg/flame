import { agentInputToContentBlocks } from "@/features/agent/adapters/wireInput";
import { localUserMessage } from "@/features/agent/adapters/optimisticUserMessage";
import { type AgentSessionStore } from "@/features/agent/adapters/agentSessionStore";
import { type Navigator } from "@/lib/navigation";
import {
  closeOpenSession,
  reconcileOpenSessions,
} from "@/features/agent/application/session/sessionSelectionModel";
import { type AgentSessionStatePort } from "@/features/agent/application/session/sessionAccess";
import { type AgentOpenSessions } from "@/features/agent/application/session/sessionAccess";
import { type AgentSessionViewPort } from "@/features/agent/application/session/sessionAccess";
import { createAgentViewSelectors } from "@/features/agent/adapters/agentViewSelectors";
import { type AgentStoreOwner } from "@/features/agent/adapters/agentStore";
export function createAgentState(
  navigation: Navigator,
  sessionStore: AgentSessionStore,
  projection: AgentStoreOwner,
) {
  const useAgentSessionStore = sessionStore.store;
  const useAgentStore = projection.store;
  const {
    getCurrentSessionView,
    useAgentAction,
    useCurrentRootRun,
    useCurrentRootRunning,
    useHasPrecedingRootRun,
    useAgentProblem,
    useAgentPlan,
    useRootNarrativeMessages,
    useTranscriptRows,
    useAgentSharedMaterial,
    useAgentOpeningCommands,
  } = createAgentViewSelectors(useAgentStore, navigation);
  function activeSessionId(): string {
    return navigation.get().session;
  }

  function getLifecycleSnapshot(): AgentOpenSessions {
    return {
      activeSessionId: activeSessionId(),
      openSessionIds: useAgentSessionStore.getState().openSessionIds,
    };
  }

  function goToSession(id: string, options?: { replace?: boolean }): void {
    const store = useAgentSessionStore.getState();
    if (id !== "") store.holdOpen(id);
    store.rememberSession(id);
    navigation.go({ session: id, view: null }, options);
  }

  const refreshOwner = projection.refresh();
  const deletedSessionListeners = new Set<(id: string) => void>();
  const closeSession = (id: string) => {
    const store = useAgentSessionStore.getState();
    const currentSessionId = activeSessionId();
    const next = closeOpenSession(
      { activeSessionId: currentSessionId, openSessionIds: store.openSessionIds },
      id,
    );
    store.release(id);
    if (next.activeSessionId === currentSessionId) {
      store.rememberSession(next.activeSessionId);
      return;
    }
    goToSession(next.activeSessionId);
  };
  const state: AgentSessionStatePort = {
    useActiveSessionId: () => navigation.use((location) => location.session),
    getActiveSessionId: activeSessionId,
    getLifecycleSnapshot,
    subscribeActiveSessionId: (onChange) =>
      navigation.subscribe((location, previous) => {
        if (location.session !== previous.session) onChange(location.session);
      }),
    subscribeLifecycle: (onChange) => {
      let last = getLifecycleSnapshot();
      const emit = () => {
        const next = getLifecycleSnapshot();
        if (
          next.activeSessionId === last.activeSessionId &&
          next.openSessionIds === last.openSessionIds
        ) {
          return;
        }
        last = next;
        onChange(next);
      };
      const unsubscribeLocation = navigation.subscribe(emit);
      const unsubscribeStore = useAgentSessionStore.subscribe(emit);
      return () => {
        unsubscribeLocation();
        unsubscribeStore();
      };
    },
    selectSession: goToSession,
    closeSession,
    deleteSession: (id) => {
      closeSession(id);
      for (const listener of deletedSessionListeners) listener(id);
    },
    subscribeDeletedSession: (listener) => {
      deletedSessionListeners.add(listener);
      return () => deletedSessionListeners.delete(listener);
    },
    reconcileSessions: (liveIds) => {
      const store = useAgentSessionStore.getState();
      const next = reconcileOpenSessions(
        {
          activeSessionId: activeSessionId(),
          openSessionIds: store.openSessionIds,
        },
        liveIds,
      );
      if (!next) return;
      store.retainOnly(next.openSessionIds);
      if (next.activeSessionId !== activeSessionId()) {
        goToSession(next.activeSessionId, { replace: true });
      } else {
        store.rememberSession(next.activeSessionId);
      }
    },
    restoreLastSession: () => {
      if (activeSessionId() !== "") return;
      const { lastSessionId } = useAgentSessionStore.getState();
      if (lastSessionId === "") return;
      goToSession(lastSessionId);
    },
  };

  const view: AgentSessionViewPort = {
    useCurrentRootRun,
    useCurrentRootRunning,
    useHasPrecedingRootRun,
    useRootNarrativeMessages,
    useTranscriptRows,
    useProblem: useAgentProblem,
    useOpeningCommands: useAgentOpeningCommands,
    usePlan: useAgentPlan,
    useSharedMaterial: useAgentSharedMaterial,
    useAction: useAgentAction,
    getCurrentView: getCurrentSessionView,
    getSessions: () => useAgentStore.getState().sessions,
    getSession: (sessionId) => useAgentStore.getState().sessions[sessionId],
    dropMessage: (sessionId, messageId) =>
      useAgentStore.getState().dropMessage(sessionId, messageId),
    projectSteerDelivery: (sessionId, messageId, command) =>
      useAgentStore.getState().projectSteerDelivery(sessionId, messageId, command),
    projectSteerReceipt: (sessionId, messageId, steer) =>
      useAgentStore.getState().projectSteerReceipt(sessionId, messageId, steer),
    reconcileMessageIdentity: (sessionId, fromId, toId, steer) =>
      useAgentStore.getState().reconcileMessageIdentity(sessionId, fromId, toId, steer),
    appendLocalUserMessage: (sessionId, messageId, input) =>
      useAgentStore
        .getState()
        .appendLocalMessage(
          sessionId,
          localUserMessage(messageId, agentInputToContentBlocks(input)),
        ),
    beginViewRefresh: (sessionId, invalidateQueuedRunEvents) =>
      refreshOwner.begin(sessionId, invalidateQueuedRunEvents),
    commitViewRefresh: (sessionId, token, view, commands) =>
      refreshOwner.commit(sessionId, token, view, commands),
    retireProjectionGeneration: (sessionIds) => refreshOwner.retireProjectionGeneration(sessionIds),
    replaceServerScope: (sessionIds) => refreshOwner.replaceServerScope(sessionIds),
    clearProblem: (sessionId) => useAgentStore.getState().clearProblem(sessionId),
    subscribeSessions: (onChange) => useAgentStore.subscribe((state) => onChange(state.sessions)),
  };
  return {
    state,
    view,
    dispose() {
      deletedSessionListeners.clear();
      refreshOwner.dispose();
    },
  };
}

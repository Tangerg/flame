import { agentInputToContentBlocks } from "./wireInput";
import { localUserMessage } from "./optimisticUserMessage";
import { useAgentSessionStore } from "./agentSessionStore";
import { navigator } from "@/lib/navigation";
import {
  closeOpenSession,
  reconcileOpenSessions,
} from "../application/session/sessionSelectionModel";
import {
  configureAgentSessionStatePort,
  type AgentOpenSessions,
} from "../application/ports/sessionState";
import { configureAgentSessionViewPort } from "../application/ports/sessionView";
import {
  getCurrentSessionView,
  useAgentAction,
  useCurrentRootRun,
  useCurrentRootRunning,
  useAgentProblem,
  useAgentPlan,
  useRootNarrativeMessages,
  useRunTree,
  useTranscriptRows,
  useAgentSessionTimeline,
  useAgentSharedMaterial,
  useAgentToolCalls,
} from "./agentViewSelectors";
import { AgentViewRefreshOwner, useAgentStore } from "./agentStore";

function activeSessionId(): string {
  return navigator().get().session;
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
  navigator().go({ session: id, view: null }, options);
}

export function installAgentStatePorts(): () => void {
  const refreshOwner = AgentViewRefreshOwner.install();
  const disposeSessionState = configureAgentSessionStatePort({
    useActiveSessionId: () => navigator().use((location) => location.session),
    getActiveSessionId: activeSessionId,
    getLifecycleSnapshot,
    subscribeActiveSessionId: (onChange) =>
      navigator().subscribe((location, previous) => {
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
      const unsubscribeLocation = navigator().subscribe(emit);
      const unsubscribeStore = useAgentSessionStore.subscribe(emit);
      return () => {
        unsubscribeLocation();
        unsubscribeStore();
      };
    },
    selectSession: goToSession,
    closeSession: (id) => {
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
    },
    useDraftSessionIds: () => useAgentSessionStore((state) => state.draftSessionIds),
    isDraftSession: (id) => useAgentSessionStore.getState().draftSessionIds.has(id),
    reconcileSessions: (liveIds) => {
      const store = useAgentSessionStore.getState();
      const next = reconcileOpenSessions(
        {
          activeSessionId: activeSessionId(),
          openSessionIds: store.openSessionIds,
          provisionalSessionIds: store.freshDraftSessionIds,
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
    markDraftSession: (id) => useAgentSessionStore.getState().markDraft(id),
  });

  const disposeViewState = configureAgentSessionViewPort({
    useCurrentRootRun,
    useCurrentRootRunning,
    useToolCalls: useAgentToolCalls,
    useSessionTimeline: useAgentSessionTimeline,
    useRootNarrativeMessages,
    useTranscriptRows,
    useRunTree,
    useProblem: useAgentProblem,
    usePlan: useAgentPlan,
    useSharedMaterial: useAgentSharedMaterial,
    useAction: useAgentAction,
    getCurrentView: getCurrentSessionView,
    getSessions: () => useAgentStore.getState().sessions,
    getSession: (sessionId) => useAgentStore.getState().sessions[sessionId],
    sendToSession: (sessionId, input, options) => {
      const send = useAgentStore.getState().sessions[sessionId]?.send;
      if (!send) return false;
      return send(input, options);
    },
    dropMessage: (sessionId, messageId) =>
      useAgentStore.getState().dropMessage(sessionId, messageId),
    reconcileMessageIdentity: (sessionId, fromId, toId, steerRunId) =>
      useAgentStore.getState().reconcileMessageIdentity(sessionId, fromId, toId, steerRunId),
    appendLocalUserMessage: (sessionId, messageId, input) =>
      useAgentStore
        .getState()
        .appendLocalMessage(
          sessionId,
          localUserMessage(messageId, agentInputToContentBlocks(input)),
        ),
    beginViewRefresh: (sessionId, invalidateQueuedRunEvents) =>
      refreshOwner.begin(sessionId, invalidateQueuedRunEvents),
    commitViewRefresh: (sessionId, token, view) => refreshOwner.commit(sessionId, token, view),
    retireProjectionGeneration: (sessionIds) => refreshOwner.retireProjectionGeneration(sessionIds),
    replaceServerScope: (sessionIds) => refreshOwner.replaceServerScope(sessionIds),
    clearProblem: (sessionId) => useAgentStore.getState().clearProblem(sessionId),
    resolveInterrupt: (sessionId, itemId, settled, resolvedAt) =>
      useAgentStore.getState().resolveInterrupt(sessionId, itemId, settled, resolvedAt),
    subscribeSessions: (onChange) => useAgentStore.subscribe((state) => onChange(state.sessions)),
  });
  return () => {
    refreshOwner.dispose();
    disposeViewState();
    disposeSessionState();
  };
}

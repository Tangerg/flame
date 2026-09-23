import type { CancelRunResponse, RunEvent, RunRef } from "@/rpc";
import type {
  AgentViewRefreshToken,
  CancelRunAction,
  ResumeRunAction,
  SendAgentInputAction,
  StopCurrentRootRunAction,
  SynchronizeSessionAction,
} from "@/plugins/builtin/agent/application/ports/sessionView";
import type { AgentProblem, AgentSessionView, Message } from "@/plugins/sdk/types/agentSessionView";
import { create } from "zustand";
import { disposeOnHmr } from "@/lib/hmr";
import { createPublicationSlot } from "@/lib/publicationSlot";
import { reduceAgentEvent } from "@/plugins/builtin/agent/application/fold/reducer";
import { foldCancelRunResponse } from "@/plugins/builtin/agent/application/fold/cancelResponse";
import { foldRunSnapshot } from "@/plugins/builtin/agent/application/fold/runSnapshot";
import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";
import {
  dismissVisibleProblem,
  dropMessage,
  reconcileMessageIdentity,
  resolveInterrupt,
  setCommandError,
  type SettledInterrupt,
} from "@/plugins/builtin/agent/application/view/viewMutations";
import { useAgentSessionStore } from "./agentSessionStore";
import { runtimeAgentEvent, runtimeCancelResult, runtimeRunFact } from "./runtimeAgentFacts";

interface SessionEntry {
  view: AgentSessionView;
  viewEpoch: bigint;
  viewRevision: bigint;
  authoritativeRevision: bigint;
  refreshSequence: bigint;
  stop: StopCurrentRootRunAction | null;
  send: SendAgentInputAction | null;
  resume: ResumeRunAction | null;
  synchronize: SynchronizeSessionAction | null;
  cancelRun: CancelRunAction | null;
}

interface AgentStore {
  sessions: Record<string, SessionEntry>;
  projectionGenerationSequence: bigint;

  applyRunEvents: (sessionId: string, events: RunEvent[]) => boolean;
  applyRunSnapshot: (sessionId: string, run: RunRef) => void;
  commitCancelResponse: (
    sessionId: string,
    expected: { viewEpoch: bigint; viewRevision: bigint },
    response: CancelRunResponse,
  ) => boolean;
  appendLocalMessage: (sessionId: string, message: Message) => void;
  ensureSession: (sessionId: string) => void;
  beginViewRefresh: (
    sessionId: string,
    invalidateQueuedRunEvents: boolean,
  ) => AgentViewRefreshToken | null;
  commitViewRefresh: (
    sessionId: string,
    token: AgentViewRefreshToken,
    view: AgentSessionView,
  ) => boolean;
  retireProjectionGeneration: (sessionIds: readonly string[]) => void;
  replaceServerScope: (sessionIds: readonly string[]) => void;
  reconcileMessageIdentity: (sessionId: string, fromId: string, toId: string) => void;
  dropMessage: (sessionId: string, id: string) => void;
  dropSession: (sessionId: string) => void;
  setStop: (sessionId: string, action: StopCurrentRootRunAction | null) => void;
  setSend: (sessionId: string, action: SendAgentInputAction | null) => void;
  setResume: (sessionId: string, action: ResumeRunAction | null) => void;
  setSynchronize: (sessionId: string, action: SynchronizeSessionAction | null) => void;
  setCancelRun: (sessionId: string, action: CancelRunAction | null) => void;
  clearProblem: (sessionId: string) => void;
  setCommandError: (sessionId: string, error: AgentProblem | null) => void;
  resolveInterrupt: (
    sessionId: string,
    itemId: string,
    settled: SettledInterrupt,
    resolvedAt: number,
  ) => void;
}

const initialProjectionCounter = 0n;

function advanceProjectionCounter(current: bigint): bigint {
  return current + 1n;
}

const emptyEntry = (viewEpoch: bigint): SessionEntry => ({
  view: EMPTY_AGENT_SESSION_VIEW,
  viewEpoch,
  viewRevision: initialProjectionCounter,
  authoritativeRevision: initialProjectionCounter,
  refreshSequence: initialProjectionCounter,
  stop: null,
  send: null,
  resume: null,
  synchronize: null,
  cancelRun: null,
});

function patchSession(
  sessions: Record<string, SessionEntry>,
  sessionId: string,
  next: Partial<SessionEntry>,
): Record<string, SessionEntry> {
  const prev = sessions[sessionId];
  if (!prev) return sessions;
  return { ...sessions, [sessionId]: { ...prev, ...next } };
}

function patchView(
  sessions: Record<string, SessionEntry>,
  sessionId: string,
  update: (view: AgentSessionView) => AgentSessionView,
): Record<string, SessionEntry> {
  const prev = sessions[sessionId];
  if (!prev) return sessions;
  const view = update(prev.view);
  if (view === prev.view) return sessions;
  return patchSession(sessions, sessionId, {
    view,
    viewRevision: advanceProjectionCounter(prev.viewRevision),
  });
}

function patchSessionState(
  state: AgentStore,
  sessionId: string,
  next: Partial<SessionEntry>,
): AgentStore | { sessions: Record<string, SessionEntry> } {
  const sessions = patchSession(state.sessions, sessionId, next);
  return sessions === state.sessions ? state : { sessions };
}

export const useAgentStore = create<AgentStore>((set) => ({
  sessions: {},
  projectionGenerationSequence: initialProjectionCounter,
  applyRunEvents: (sessionId, events) => {
    let applied = false;
    set((state) => {
      if (events.length === 0) return state;
      const prev = state.sessions[sessionId];
      if (!prev) return state;
      let view = prev.view;
      for (const event of events) view = reduceAgentEvent(view, runtimeAgentEvent(event));
      applied = true;
      if (view === prev.view) return state;
      return {
        sessions: patchSession(state.sessions, sessionId, {
          view,
          viewRevision: advanceProjectionCounter(prev.viewRevision),
        }),
      };
    });
    return applied;
  },
  applyRunSnapshot: (sessionId, run) =>
    set((state) => {
      const sessions = patchView(state.sessions, sessionId, (view) =>
        foldRunSnapshot(view, runtimeRunFact(run)),
      );
      return sessions === state.sessions ? state : { sessions };
    }),
  commitCancelResponse: (sessionId, expected, response) => {
    let committed = false;
    set((state) => {
      const entry = state.sessions[sessionId];
      if (
        !entry ||
        entry.viewEpoch !== expected.viewEpoch ||
        entry.viewRevision !== expected.viewRevision
      ) {
        return state;
      }
      const sessions = patchView(state.sessions, sessionId, (view) =>
        foldCancelRunResponse(view, runtimeCancelResult(response)),
      );
      committed = true;
      return sessions === state.sessions ? state : { sessions };
    });
    return committed;
  },
  appendLocalMessage: (sessionId, message) =>
    set((state) => {
      const sessions = patchView(state.sessions, sessionId, (view) =>
        view.messages.some((existing) => existing.id === message.id)
          ? view
          : { ...view, messages: [...view.messages, message] },
      );
      return sessions === state.sessions ? state : { sessions };
    }),
  ensureSession: (sessionId) =>
    set((state) => {
      if (state.sessions[sessionId]) return state;
      const viewEpoch = advanceProjectionCounter(state.projectionGenerationSequence);
      return {
        sessions: { ...state.sessions, [sessionId]: emptyEntry(viewEpoch) },
        projectionGenerationSequence: viewEpoch,
      };
    }),
  beginViewRefresh: (sessionId, invalidateQueuedRunEvents) => {
    let token: AgentViewRefreshToken | null = null;
    set((state) => {
      const entry = state.sessions[sessionId];
      if (!entry) return state;
      const requestSequence = advanceProjectionCounter(entry.refreshSequence);
      const viewEpoch = invalidateQueuedRunEvents
        ? advanceProjectionCounter(state.projectionGenerationSequence)
        : entry.viewEpoch;
      token = {
        generation: viewEpoch,
        requestSequence,
        viewRevision: entry.viewRevision,
      };
      const sessions = patchSession(state.sessions, sessionId, {
        refreshSequence: requestSequence,
        viewEpoch,
      });
      return {
        sessions,
        ...(invalidateQueuedRunEvents ? { projectionGenerationSequence: viewEpoch } : {}),
      };
    });
    return token;
  },
  commitViewRefresh: (sessionId, token, view) => {
    let committed = false;
    set((state) => {
      const entry = state.sessions[sessionId];
      if (
        !entry ||
        entry.viewEpoch !== token.generation ||
        entry.refreshSequence !== token.requestSequence ||
        entry.viewRevision !== token.viewRevision
      ) {
        return state;
      }
      committed = true;
      return patchSessionState(state, sessionId, {
        view,
        viewRevision: advanceProjectionCounter(entry.viewRevision),
        authoritativeRevision: advanceProjectionCounter(entry.authoritativeRevision),
      });
    });
    return committed;
  },
  retireProjectionGeneration: (sessionIds) =>
    set((state) => {
      const viewEpoch = advanceProjectionCounter(state.projectionGenerationSequence);
      let sessions = state.sessions;
      for (const sessionId of new Set(sessionIds)) {
        const entry = sessions[sessionId];
        if (!entry) continue;
        sessions = patchSession(sessions, sessionId, {
          refreshSequence: advanceProjectionCounter(entry.refreshSequence),
          viewEpoch,
        });
      }
      return sessions === state.sessions
        ? state
        : { sessions, projectionGenerationSequence: viewEpoch };
    }),
  replaceServerScope: (sessionIds) =>
    set((state) => {
      const viewEpoch = advanceProjectionCounter(state.projectionGenerationSequence);
      let sessions = state.sessions;
      for (const sessionId of new Set(sessionIds)) {
        const entry = sessions[sessionId];
        if (!entry) continue;
        sessions = patchSession(sessions, sessionId, {
          view: EMPTY_AGENT_SESSION_VIEW,
          viewEpoch,
          viewRevision: advanceProjectionCounter(entry.viewRevision),
          authoritativeRevision: initialProjectionCounter,
          refreshSequence: advanceProjectionCounter(entry.refreshSequence),
        });
      }
      return sessions === state.sessions
        ? state
        : { sessions, projectionGenerationSequence: viewEpoch };
    }),
  reconcileMessageIdentity: (sessionId, fromId, toId) =>
    set((state) => {
      const sessions = patchView(state.sessions, sessionId, (view) =>
        reconcileMessageIdentity(view, fromId, toId),
      );
      return sessions === state.sessions ? state : { sessions };
    }),
  dropMessage: (sessionId, id) =>
    set((state) => {
      const sessions = patchView(state.sessions, sessionId, (view) => dropMessage(view, id));
      return sessions === state.sessions ? state : { sessions };
    }),
  dropSession: (sessionId) =>
    set((state) => {
      if (!(sessionId in state.sessions)) return state;
      const next = { ...state.sessions };
      delete next[sessionId];
      return { sessions: next };
    }),
  setStop: (sessionId, action) =>
    set((state) => patchSessionState(state, sessionId, { stop: action })),
  setSend: (sessionId, action) =>
    set((state) => patchSessionState(state, sessionId, { send: action })),
  setResume: (sessionId, action) =>
    set((state) => patchSessionState(state, sessionId, { resume: action })),
  setSynchronize: (sessionId, action) =>
    set((state) => patchSessionState(state, sessionId, { synchronize: action })),
  setCancelRun: (sessionId, action) =>
    set((state) => patchSessionState(state, sessionId, { cancelRun: action })),
  clearProblem: (sessionId) =>
    set((state) => {
      const sessions = patchView(state.sessions, sessionId, dismissVisibleProblem);
      return sessions === state.sessions ? state : { sessions };
    }),
  setCommandError: (sessionId, error) =>
    set((state) => {
      const sessions = patchView(state.sessions, sessionId, (view) => setCommandError(view, error));
      return sessions === state.sessions ? state : { sessions };
    }),
  resolveInterrupt: (sessionId, itemId, settled, resolvedAt) =>
    set((state) => {
      const sessions = patchView(state.sessions, sessionId, (view) =>
        resolveInterrupt(view, itemId, settled, resolvedAt),
      );
      return sessions === state.sessions ? state : { sessions };
    }),
}));

export class AgentViewRefreshOwner {
  #disposed = false;

  private constructor() {}

  static install(): AgentViewRefreshOwner {
    const owner = new AgentViewRefreshOwner();
    agentViewRefreshPublication.publish(owner, (predecessor) => predecessor.dispose());
    return owner;
  }

  begin(sessionId: string, invalidateQueuedRunEvents: boolean): AgentViewRefreshToken | null {
    if (!this.#ownsGeneration()) return null;
    return useAgentStore.getState().beginViewRefresh(sessionId, invalidateQueuedRunEvents);
  }

  commit(sessionId: string, token: AgentViewRefreshToken, view: AgentSessionView): boolean {
    if (!this.#ownsGeneration()) return false;
    return useAgentStore.getState().commitViewRefresh(sessionId, token, view);
  }

  retireProjectionGeneration(sessionIds: readonly string[]): void {
    if (!this.#ownsGeneration()) return;
    useAgentStore.getState().retireProjectionGeneration(sessionIds);
  }

  replaceServerScope(sessionIds: readonly string[]): void {
    if (!this.#ownsGeneration()) return;
    useAgentStore.getState().replaceServerScope(sessionIds);
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    agentViewRefreshPublication.withdraw(this);
  }

  #ownsGeneration(): boolean {
    return !this.#disposed && agentViewRefreshPublication.owns(this);
  }
}

const agentViewRefreshPublication = createPublicationSlot<AgentViewRefreshOwner>();

const unsubPruneSessions = useAgentSessionStore.subscribe((state, prev) => {
  if (state.openSessionIds === prev.openSessionIds) return;
  const live = new Set(state.openSessionIds);
  const sessions = useAgentStore.getState().sessions;
  for (const id of Object.keys(sessions)) {
    if (!live.has(id)) useAgentStore.getState().dropSession(id);
  }
});

disposeOnHmr(unsubPruneSessions);

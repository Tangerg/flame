import {
  projectRootRunSettlement,
  type RootRunSettlement,
} from "@/features/agent/application/run/rootAttention";
import { type CancelRunResponse, type RunEvent, type RunRef } from "@flame/runtime-client";
import {
  type AgentViewRefreshToken,
  type CancelRunAction,
  type ResumeRunAction,
  type RetryOpeningAction,
  type SendAgentInputAction,
  type StopCurrentRootRunAction,
  type SynchronizeSessionAction,
} from "@/features/agent/application/session/sessionAccess";
import {
  type AgentProblem,
  type AgentSessionView,
  type Message,
} from "@/features/agent/public/sessionView";
import { create } from "zustand";
import { createPublicationSlot } from "@/lib/publicationSlot";
import { reduceAgentEvent } from "@/features/agent/application/fold/reducer";
import { foldCancelRunResponse } from "@/features/agent/application/fold/cancelResponse";
import { foldRunSnapshot } from "@/features/agent/application/fold/runSnapshot";
import { EMPTY_AGENT_SESSION_VIEW } from "@/features/agent/public/sessionView";
import {
  dropMessage,
  reconcileMessageIdentity,
  projectSteerReceipt,
  projectSteerDelivery,
} from "@/features/agent/application/view/viewMutations";
import {
  dismissVisibleProblem,
  EMPTY_PROBLEM_PRESENTATION,
  type ProblemPresentation,
  withCommandError,
} from "@/features/agent/application/view/problemPresentation";
import { type AgentSessionStore } from "@/features/agent/adapters/agentSessionStore";
import {
  runtimeAgentEvent,
  runtimeCancelResult,
  runtimeRunFact,
} from "@/features/agent/adapters/runtimeAgentFacts";
import { type AgentOpeningCommand } from "@/features/agent/application/runtimeGateway";
interface SessionEntry {
  view: AgentSessionView;
  // problem is presentation the user and local commands own; an authoritative
  // refresh replaces view but never this.
  problem: ProblemPresentation;
  viewEpoch: bigint;
  viewRevision: bigint;
  authoritativeRevision: bigint;
  refreshSequence: bigint;
  stop: StopCurrentRootRunAction | null;
  send: SendAgentInputAction | null;
  resume: ResumeRunAction | null;
  synchronize: SynchronizeSessionAction | null;
  cancelRun: CancelRunAction | null;
  openingCommands: readonly AgentOpeningCommand[];
  retryOpening: RetryOpeningAction | null;
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
    openingCommands?: readonly AgentOpeningCommand[],
  ) => boolean;
  retireProjectionGeneration: (sessionIds: readonly string[]) => void;
  replaceServerScope: (sessionIds: readonly string[]) => void;
  reconcileMessageIdentity: (
    sessionId: string,
    fromId: string,
    toId: string,
    steer?: Required<Pick<NonNullable<Message["steer"]>, "runId" | "receipt">>,
  ) => void;
  dropMessage: (sessionId: string, id: string) => void;
  projectSteerReceipt: (
    sessionId: string,
    messageId: string,
    steer: Required<Pick<NonNullable<Message["steer"]>, "runId" | "receipt">>,
  ) => void;
  projectSteerDelivery: (
    sessionId: string,
    messageId: string,
    command: Pick<NonNullable<Message["steer"]>, "runId" | "delivery">,
  ) => void;
  dropSession: (sessionId: string) => void;
  setStop: (sessionId: string, action: StopCurrentRootRunAction | null) => void;
  setSend: (sessionId: string, action: SendAgentInputAction | null) => void;
  setResume: (sessionId: string, action: ResumeRunAction | null) => void;
  setSynchronize: (sessionId: string, action: SynchronizeSessionAction | null) => void;
  setCancelRun: (sessionId: string, action: CancelRunAction | null) => void;
  setRetryOpening: (sessionId: string, action: RetryOpeningAction | null) => void;
  projectOpeningAccepted: (sessionId: string, idempotencyKey: string) => void;
  clearProblem: (sessionId: string) => void;
  setCommandError: (sessionId: string, error: AgentProblem | null) => void;
}

const initialProjectionCounter = 0n;

function advanceProjectionCounter(current: bigint): bigint {
  return current + 1n;
}

const emptyEntry = (viewEpoch: bigint): SessionEntry => ({
  view: EMPTY_AGENT_SESSION_VIEW,
  problem: EMPTY_PROBLEM_PRESENTATION,
  viewEpoch,
  viewRevision: initialProjectionCounter,
  authoritativeRevision: initialProjectionCounter,
  refreshSequence: initialProjectionCounter,
  stop: null,
  send: null,
  resume: null,
  synchronize: null,
  cancelRun: null,
  openingCommands: [],
  retryOpening: null,
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

export function createAgentStore(sessionStore: AgentSessionStore) {
  const useAgentSessionStore = sessionStore.store;
  const settlementListeners = new Set<(settlement: RootRunSettlement) => void>();
  let disposed = false;
  function publishSettlements(generation: bigint, settlements: RootRunSettlement[]): void {
    for (const settlement of settlements) {
      for (const listener of [...settlementListeners]) {
        if (
          disposed ||
          useAgentStore.getState().sessions[settlement.sessionId]?.viewEpoch !== generation
        )
          return;
        if (settlementListeners.has(listener)) listener(settlement);
      }
    }
  }
  const useAgentStore = create<AgentStore>((set) => ({
    sessions: {},
    projectionGenerationSequence: initialProjectionCounter,
    applyRunEvents: (sessionId, events) => {
      let applied = false;
      let generation = initialProjectionCounter;
      const settlements: RootRunSettlement[] = [];
      set((state) => {
        if (events.length === 0) return state;
        const prev = state.sessions[sessionId];
        if (!prev) return state;
        generation = prev.viewEpoch;
        let view = prev.view;
        let problem = prev.problem;
        for (const event of events) {
          const agentEvent = runtimeAgentEvent(event);
          const next = reduceAgentEvent(view, agentEvent);
          const settlement = projectRootRunSettlement(
            view,
            next,
            agentEvent.runId,
            agentEvent.segmentId,
          );
          if (settlement) settlements.push(settlement);
          view = next;
          // A started segment supersedes the command failure that preceded it.
          if (agentEvent.event.type === "segment.started")
            problem = withCommandError(problem, null);
        }
        applied = true;
        if (view === prev.view && problem === prev.problem) return state;
        return {
          sessions: patchSession(state.sessions, sessionId, {
            view,
            problem,
            viewRevision:
              view === prev.view ? prev.viewRevision : advanceProjectionCounter(prev.viewRevision),
          }),
        };
      });
      // Rejected batches never publish partial lifecycle effects.
      if (applied) publishSettlements(generation, settlements);
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
      let settlement: RootRunSettlement | null = null;
      set((state) => {
        const entry = state.sessions[sessionId];
        if (
          !entry ||
          entry.viewEpoch !== expected.viewEpoch ||
          entry.viewRevision !== expected.viewRevision
        ) {
          return state;
        }
        const sessions = patchView(state.sessions, sessionId, (view) => {
          const next = foldCancelRunResponse(view, runtimeCancelResult(response));
          const rootId = response.type === "root" ? response.run.id : response.rootRun.id;
          settlement = projectRootRunSettlement(
            view,
            next,
            rootId,
            view.runsById[rootId]?.activeSegmentId ?? null,
          );
          return next;
        });
        committed = true;
        return sessions === state.sessions ? state : { sessions };
      });
      if (committed && settlement) publishSettlements(expected.viewEpoch, [settlement]);
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
    commitViewRefresh: (sessionId, token, view, openingCommands) => {
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
          ...(openingCommands ? { openingCommands } : {}),
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
            openingCommands: [],
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
            openingCommands: [],
            problem: EMPTY_PROBLEM_PRESENTATION,
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
    reconcileMessageIdentity: (sessionId, fromId, toId, steer) =>
      set((state) => {
        const sessions = patchView(state.sessions, sessionId, (view) =>
          reconcileMessageIdentity(view, fromId, toId, steer),
        );
        return sessions === state.sessions ? state : { sessions };
      }),
    dropMessage: (sessionId, id) =>
      set((state) => {
        const sessions = patchView(state.sessions, sessionId, (view) => dropMessage(view, id));
        return sessions === state.sessions ? state : { sessions };
      }),
    projectSteerReceipt: (sessionId, messageId, steer) =>
      set((state) => {
        const sessions = patchView(state.sessions, sessionId, (view) =>
          projectSteerReceipt(view, messageId, steer),
        );
        return sessions === state.sessions ? state : { sessions };
      }),
    projectSteerDelivery: (sessionId, messageId, command) =>
      set((state) => {
        const sessions = patchView(state.sessions, sessionId, (view) =>
          projectSteerDelivery(view, messageId, command),
        );
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
    setRetryOpening: (sessionId, action) =>
      set((state) => patchSessionState(state, sessionId, { retryOpening: action })),
    projectOpeningAccepted: (sessionId, idempotencyKey) =>
      set((state) => {
        const entry = state.sessions[sessionId];
        if (!entry) return state;
        return patchSessionState(state, sessionId, {
          openingCommands: entry.openingCommands.filter(
            (command) => command.idempotencyKey !== idempotencyKey,
          ),
          problem: withCommandError(entry.problem, null),
          viewRevision: advanceProjectionCounter(entry.viewRevision),
        });
      }),
    setSynchronize: (sessionId, action) =>
      set((state) => patchSessionState(state, sessionId, { synchronize: action })),
    setCancelRun: (sessionId, action) =>
      set((state) => patchSessionState(state, sessionId, { cancelRun: action })),
    clearProblem: (sessionId) =>
      set((state) => {
        const entry = state.sessions[sessionId];
        if (!entry) return state;
        return patchSessionState(state, sessionId, {
          problem: dismissVisibleProblem(entry.view, entry.problem),
        });
      }),
    setCommandError: (sessionId, error) =>
      set((state) => {
        const entry = state.sessions[sessionId];
        if (!entry) return state;
        return patchSessionState(state, sessionId, {
          problem: withCommandError(entry.problem, error),
        });
      }),
  }));

  class AgentViewRefreshOwner {
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

    commit(
      sessionId: string,
      token: AgentViewRefreshToken,
      view: AgentSessionView,
      openingCommands?: readonly AgentOpeningCommand[],
    ): boolean {
      if (!this.#ownsGeneration()) return false;
      return useAgentStore.getState().commitViewRefresh(sessionId, token, view, openingCommands);
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

  return {
    store: useAgentStore,
    refresh: AgentViewRefreshOwner.install,
    subscribeRootRunSettlements(listener: (settlement: RootRunSettlement) => void): () => void {
      if (disposed) return () => undefined;
      settlementListeners.add(listener);
      return () => {
        settlementListeners.delete(listener);
      };
    },
    dispose() {
      disposed = true;
      settlementListeners.clear();
      unsubPruneSessions();
      agentViewRefreshPublication.current()?.dispose();
    },
  };
}
export type AgentStoreOwner = ReturnType<typeof createAgentStore>;

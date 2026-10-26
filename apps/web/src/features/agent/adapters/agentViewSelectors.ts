import { useMemo } from "react";
import { useShallow } from "zustand/react/shallow";
import { type Navigator } from "@/lib/navigation";
import {
  type AgentPlan,
  type AgentProblem,
  type AgentSessionView,
  type Message,
} from "@/features/agent/public/sessionView";
import { EMPTY_AGENT_SESSION_VIEW } from "@/features/agent/public/sessionView";
import {
  selectCurrentRootRun,
  selectRootNarrativeMessages,
} from "@/features/agent/application/view/runTree";
import {
  EMPTY_PROBLEM_PRESENTATION,
  selectVisibleProblem,
} from "@/features/agent/application/view/problemPresentation";
import {
  buildTranscriptRows,
  EMPTY_TRANSCRIPT_ROW_CACHE,
  type TranscriptRow,
  type TranscriptRowCache,
} from "@/features/agent/application/conversation/transcriptRows";
import {
  type AgentProjectionMaterial,
  type SendAgentInputAction,
  type StopCurrentRootRunAction,
} from "@/features/agent/application/session/sessionAccess";
import { type AgentStoreOwner } from "@/features/agent/adapters/agentStore";
import { type AgentOpeningCommand } from "@/features/agent/application/runtimeGateway";
const NO_OPENING_COMMANDS: readonly AgentOpeningCommand[] = [];

export function createAgentViewSelectors(
  useAgentStore: AgentStoreOwner["store"],
  navigation: Navigator,
) {
  function useAgentOpeningCommands(): readonly AgentOpeningCommand[] {
    const sessionId = navigation.use((location) => location.session);
    return useAgentStore(
      (state) => state.sessions[sessionId]?.openingCommands ?? NO_OPENING_COMMANDS,
    );
  }

  function useActiveAgentView<T>(select: (view: AgentSessionView) => T): T {
    const sessionId = navigation.use((location) => location.session);
    return useAgentStore((state) =>
      select(state.sessions[sessionId]?.view ?? EMPTY_AGENT_SESSION_VIEW),
    );
  }

  type AgentStoreState = ReturnType<typeof useAgentStore.getState>;

  class TranscriptRowsProjection {
    private cache: TranscriptRowCache = EMPTY_TRANSCRIPT_ROW_CACHE;
    private rows: readonly TranscriptRow[] = [];

    constructor(private readonly sessionId: string) {}

    select(state: AgentStoreState): readonly TranscriptRow[] {
      const view = state.sessions[this.sessionId]?.view ?? EMPTY_AGENT_SESSION_VIEW;
      const built = buildTranscriptRows(view, this.cache);
      this.cache = built.cache;
      if (
        built.rows.length === this.rows.length &&
        built.rows.every((row, index) => row === this.rows[index])
      ) {
        return this.rows;
      }
      this.rows = built.rows;
      return this.rows;
    }
  }

  function useCurrentRootRun() {
    return useActiveAgentView(selectCurrentRootRun);
  }

  function useCurrentRootRunning(): boolean {
    return useActiveAgentView((view) => selectCurrentRootRun(view)?.status === "running");
  }

  function useAgentAction(kind: "stop"): StopCurrentRootRunAction | null;

  function useAgentAction(kind: "send"): SendAgentInputAction | null;

  function useAgentAction(
    kind: "stop" | "send",
  ): StopCurrentRootRunAction | SendAgentInputAction | null {
    const sessionId = navigation.use((location) => location.session);
    return useAgentStore((state) => state.sessions[sessionId]?.[kind] ?? null);
  }

  function useRootNarrativeMessages(): Message[] {
    const view = useActiveAgentView((current) => current);
    return useMemo(() => selectRootNarrativeMessages(view), [view]);
  }

  function useTranscriptRows(): readonly TranscriptRow[] {
    const sessionId = navigation.use((location) => location.session);
    const selectRows = useMemo(() => {
      const projection = new TranscriptRowsProjection(sessionId);
      return (state: AgentStoreState): readonly TranscriptRow[] => projection.select(state);
    }, [sessionId]);
    return useAgentStore(selectRows);
  }

  function useAgentProblem(): AgentProblem | null {
    const sessionId = navigation.use((location) => location.session);
    return useAgentStore((state) => {
      const entry = state.sessions[sessionId];
      return entry
        ? selectVisibleProblem(entry.view, entry.problem)
        : selectVisibleProblem(EMPTY_AGENT_SESSION_VIEW, EMPTY_PROBLEM_PRESENTATION);
    });
  }

  function useAgentPlan(): AgentProjectionMaterial<AgentPlan> {
    const sessionId = navigation.use((location) => location.session);
    return useAgentStore(
      useShallow((state) => {
        const entry = state.sessions[sessionId];
        return {
          generation: entry?.viewEpoch ?? 0n,
          value: entry?.view.plan ?? undefined,
        };
      }),
    );
  }

  function useAgentSharedMaterial<T = unknown>(path?: string): AgentProjectionMaterial<T> {
    const sessionId = navigation.use((location) => location.session);
    return useAgentStore(
      useShallow((state) => {
        const entry = state.sessions[sessionId];
        return {
          generation: entry?.viewEpoch ?? 0n,
          value: selectFromShared<T>(entry?.view.shared ?? EMPTY_AGENT_SESSION_VIEW.shared, path),
        };
      }),
    );
  }

  function getCurrentSessionView(): AgentSessionView {
    const sessionId = navigation.get().session;
    return useAgentStore.getState().sessions[sessionId]?.view ?? EMPTY_AGENT_SESSION_VIEW;
  }

  function selectFromShared<T>(
    shared: Record<string, unknown>,
    path: string | undefined,
  ): T | undefined {
    let current: unknown = shared;
    if (!path) return current as T;
    for (const segment of path.split(".")) {
      if (current == null || typeof current !== "object") return undefined;
      current = (current as Record<string, unknown>)[segment];
    }
    return current as T;
  }

  return {
    useAgentOpeningCommands,
    useCurrentRootRun,
    useCurrentRootRunning,
    useAgentAction,
    useRootNarrativeMessages,
    useTranscriptRows,
    useAgentProblem,
    useAgentPlan,
    useAgentSharedMaterial,
    getCurrentSessionView,
  };
}

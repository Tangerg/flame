import type { ForkWorktreeConfirmation } from "@flame/runtime-client/wire";
import { useAgentDependencies } from "@/features/agent/application/agentContext";
import { type AgentDependencies } from "@/features/agent/application/agentDependencies";
import { useCallback } from "react";
import { invalidateAgentSessions } from "@/features/agent/application/session/sessionQueries";

import { reportSessionError } from "@/features/agent/application/session/reportSessionError";

export function forkSessionAt(
  agentAccess: AgentDependencies,
  id: string,
  fromRunId?: string,
): Promise<void> {
  const owner = agentAccess.commands.current();
  const runtime = agentAccess.runtime;
  const state = agentAccess.state;
  const key = fromRunId ? `${id}:${fromRunId}` : id;
  return owner
    .runSessionFork(key, async () => {
      const sessionId = await runtime.forkSession({ sessionId: id, fromRunId });
      owner.assertCurrent();
      state.selectSession(sessionId);
      invalidateAgentSessions([sessionId]);
    })
    .catch((error: unknown) => {
      if (owner.isCurrent()) reportSessionError(agentAccess.notifications, "fork", error);
    });
}

export function useForkSession(): (id: string) => Promise<void> {
  const agentAccess = useAgentDependencies();

  return useCallback((id) => forkSessionAt(agentAccess, id), [agentAccess]);
}

export function useSessionWorktreeFork() {
  const agent = useAgentDependencies();
  return {
    preview: useCallback(
      (id: string, signal?: AbortSignal) => {
        const owner = agent.commands.current();
        return owner.settle(agent.runtime.previewSessionFork(id, signal));
      },
      [agent],
    ),
    confirm: useCallback(
      (id: string, worktree: ForkWorktreeConfirmation) => {
        const owner = agent.commands.current();
        return owner.runSessionFork(worktree.id, async () => {
          const fork = await agent.runtime.forkSession({ sessionId: id, worktree });
          owner.assertCurrent();
          agent.state.selectSession(fork);
          invalidateAgentSessions([fork]);
        });
      },
      [agent],
    ),
  };
}

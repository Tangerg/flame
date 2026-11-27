import { useAgentDependencies } from "@/features/agent/application/agentContext";
import { type AgentDependencies } from "@/features/agent/application/agentDependencies";
import { queryClient } from "@/lib/queryClient";
import { useCallback } from "react";
import {
  AGENT_SESSIONS_KEY,
  invalidateAgentSessions,
  type AgentSessionSummary,
} from "@/features/agent/application/session/sessionQueries";
import { type AgentRuntimeGateway } from "@/features/agent/application/runtimeGateway";
import { type AgentSessionStatePort } from "@/features/agent/application/session/sessionAccess";
import { reportSessionError } from "@/features/agent/application/session/reportSessionError";
import { type AgentCommandOwner } from "@/features/agent/application/agentCommandOwner";
interface CreateSessionOptions {
  cwd: string;
  projectId?: string;
}

async function createAndOpen({
  owner,
  runtime,
  state,
  cwd,
  projectId,
}: CreateSessionOptions & {
  owner: AgentCommandOwner;
  runtime: AgentRuntimeGateway;
  state: AgentSessionStatePort;
}): Promise<string> {
  const sessionId = await owner.settle(runtime.createSession({ cwd, projectId }));
  owner.assertCurrent();
  await owner.settle(invalidateAgentSessions());
  owner.assertCurrent();
  state.selectSession(sessionId);
  return sessionId;
}

function joinKey(opts: CreateSessionOptions): string {
  return JSON.stringify([opts.cwd, opts.projectId ?? null]);
}

function doCreate(
  agentAccess: AgentDependencies,
  opts: CreateSessionOptions,
): Promise<string | null> {
  if (opts.cwd.trim() === "") return Promise.resolve(null);
  const owner = agentAccess.commands.current();
  const runtime = agentAccess.runtime;
  const state = agentAccess.state;
  const key = joinKey(opts);
  return owner
    .runSessionCreate(key, () => createAndOpen({ owner, runtime, state, ...opts }))
    .catch((error: unknown) => {
      if (owner.isCurrent()) reportSessionError(agentAccess.notifications, "create", error);
      return null;
    });
}

export function createSession(agentAccess: AgentDependencies): Promise<string | null> {
  const sessionId = agentAccess.state.getActiveSessionId();
  if (!sessionId) return Promise.resolve(null);
  const sessions = queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY]);
  const active = sessions?.find((session) => session.id === sessionId);
  if (!active) return Promise.resolve(null);
  const current = agentAccess.view.getSession(sessionId);
  if (
    current?.hasAuthoritativeSnapshot &&
    current.openingCommands.length === 0 &&
    current.view.messages.length === 0 &&
    Object.keys(current.view.runsById).length === 0
  )
    return Promise.resolve(sessionId);
  return doCreate(agentAccess, { cwd: active.workspace.path, projectId: active.projectId });
}

export function useCreateSession(): (opts: CreateSessionOptions) => Promise<string | null> {
  const agentAccess = useAgentDependencies();

  return useCallback((opts) => doCreate(agentAccess, opts), [agentAccess]);
}

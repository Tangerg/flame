import { useAgentDependencies } from "@/features/agent/application/agentContext";
import { type AgentDependencies } from "@/features/agent/application/agentDependencies";
import { queryClient } from "@/lib/queryClient";
import { useCallback } from "react";
import {
  AGENT_SESSIONS_KEY,
  commitCreatedAgentSession,
  type AgentSessionSummary,
} from "@/features/agent/application/session/sessionQueries";
import { type AgentRuntimeGateway } from "@/features/agent/application/runtimeGateway";
import { type AgentSessionStatePort } from "@/features/agent/application/session/sessionAccess";
import { type AgentSessionViewPort } from "@/features/agent/application/session/sessionAccess";
import { reportSessionError } from "@/features/agent/application/session/reportSessionError";
import { type AgentCommandOwner } from "@/features/agent/application/agentCommandOwner";
export interface CreateSessionOptions {
  cwd: string;
  reuseEmptySession?: boolean;
}

async function createAndOpen({
  owner,
  runtime,
  state,
  cwd,
}: CreateSessionOptions & {
  owner: AgentCommandOwner;
  runtime: AgentRuntimeGateway;
  state: AgentSessionStatePort;
}): Promise<string> {
  const session = await owner.settle(runtime.createSession({ cwd }));
  owner.assertCurrent();
  commitCreatedAgentSession(session);
  state.selectSession(session.id);
  return session.id;
}

function joinKey(opts: CreateSessionOptions): string {
  return `cwd:${opts.cwd}`;
}

function alreadyOnAnEmptySession(
  opts: CreateSessionOptions,
  state: AgentSessionStatePort,
  view: AgentSessionViewPort,
): string | null {
  if (!opts.reuseEmptySession) return null;
  const sessionId = state.getActiveSessionId();
  if (!sessionId) return null;
  const loaded = view.getSession(sessionId)?.view;
  return loaded && loaded.messages.length === 0 ? sessionId : null;
}

function doCreate(
  agentAccess: AgentDependencies,
  opts: CreateSessionOptions,
): Promise<string | null> {
  if (opts.cwd.trim() === "") return Promise.resolve(null);
  const owner = agentAccess.commands.current();
  const runtime = agentAccess.runtime;
  const state = agentAccess.state;
  const view = agentAccess.view;
  const key = joinKey(opts);
  const fresh = alreadyOnAnEmptySession(opts, state, view);
  if (fresh) return Promise.resolve(fresh);
  return owner
    .runSessionCreate(key, () => createAndOpen({ owner, runtime, state, ...opts }))
    .catch((error: unknown) => {
      if (owner.isCurrent()) reportSessionError("create", error);
      return null;
    });
}

export function createSession(agentAccess: AgentDependencies): Promise<string | null> {
  const sessionId = agentAccess.state.getActiveSessionId();
  if (!sessionId) return Promise.resolve(null);
  const sessions = queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY]);
  const cwd = sessions?.find((session) => session.id === sessionId)?.workspace.path;
  if (!cwd || cwd.trim() === "") return Promise.resolve(null);
  return doCreate(agentAccess, { cwd, reuseEmptySession: true });
}

export function useCreateSession(): (opts: CreateSessionOptions) => Promise<string | null> {
  const agentAccess = useAgentDependencies();

  return useCallback((opts) => doCreate(agentAccess, opts), [agentAccess]);
}

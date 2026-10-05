import { queryClient } from "@/lib/queryClient";
import { useCallback } from "react";
import {
  AGENT_SESSIONS_KEY,
  commitCreatedAgentSession,
  type AgentSessionSummary,
} from "./sessionQueries";
import { agentRuntime, type AgentRuntimeGateway } from "../ports/runtimeGateway";
import { agentSessionState, type AgentSessionStatePort } from "../ports/sessionState";
import { agentSessionView, type AgentSessionViewPort } from "../ports/sessionView";
import { reportSessionError } from "./reportSessionError";
import { agentCommandOwner, type AgentCommandOwner } from "../agentCommandOwner";

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

function doCreate(opts: CreateSessionOptions): Promise<string | null> {
  if (opts.cwd.trim() === "") return Promise.resolve(null);
  const owner = agentCommandOwner();
  const runtime = agentRuntime();
  const state = agentSessionState();
  const view = agentSessionView();
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

export function createSession(): Promise<string | null> {
  const sessionId = agentSessionState().getActiveSessionId();
  if (!sessionId) return Promise.resolve(null);
  const sessions = queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY]);
  const cwd = sessions?.find((session) => session.id === sessionId)?.workspace.path;
  if (!cwd || cwd.trim() === "") return Promise.resolve(null);
  return doCreate({ cwd, reuseEmptySession: true });
}

export function useCreateSession(): (opts: CreateSessionOptions) => Promise<string | null> {
  return useCallback((opts) => doCreate(opts), []);
}

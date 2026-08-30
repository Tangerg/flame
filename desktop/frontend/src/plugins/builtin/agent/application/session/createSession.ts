import { queryClient } from "@/lib/queryClient";
import { useCallback } from "react";
import {
  AGENT_SESSIONS_KEY,
  invalidateAgentSessions,
  type AgentSessionSummary,
} from "./sessionQueries";
import { agentRuntime, type AgentRuntimeGateway } from "../ports/runtimeGateway";
import { agentSessionState, type AgentSessionStatePort } from "../ports/sessionState";
import { agentSessionView, type AgentSessionViewPort } from "../ports/sessionView";
import { reportSessionError } from "./reportSessionError";
import { agentCommandOwner, type AgentCommandOwner } from "../agentCommandOwner";

export interface CreateSessionOptions {
  /** Required: Desktop never delegates an omitted workspace to the Runtime default,
   *  because project selection is an explicit user gesture. */
  cwd: string;
  /** Only the top-level New action may set this: it already knows the cwd belongs to the
   *  active Session, which project-row creation does not. */
  reuseFreshDraft?: boolean;
}

/**
 * A draft is a REAL session — `runs.start` works immediately — that stays out of the
 * visible Work Index until its first message graduates it. Returns the new id, or null if
 * the create failed.
 */
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
  const session = await runtime.createSession({ cwd });
  owner.assertCurrent();
  // Marked BEFORE selecting, so the mounted lifecycle can skip a durable read for this
  // same-process empty identity.
  state.markDraftSession(session.id);
  state.selectSession(session.id); // opens + sets active → remounts chat
  // A cwd create may also have minted a brand-new project.
  void invalidateAgentSessions();
  return session.id;
}

// Only EXACT workspace destinations may share an in-flight create: requests for different
// projects must never receive one another's Session identity.
function joinKey(opts: CreateSessionOptions): string {
  return `cwd:${opts.cwd}`;
}

/**
 * "New session" is a DESTINATION, not an instruction to allocate, so it is a no-op in front
 * of an empty composer. Only a DRAFT counts: an ordinary session also reads as message-less
 * while its history loads, and reusing it drops the user back where they asked to leave.
 */
function alreadyOnAFreshSession(
  opts: CreateSessionOptions,
  state: AgentSessionStatePort,
  view: AgentSessionViewPort,
): string | null {
  if (!opts.reuseFreshDraft) return null;
  const sessionId = state.getActiveSessionId();
  if (!sessionId || !state.isDraftSession(sessionId)) return null;
  const messages = view.getSession(sessionId)?.view.messages ?? [];
  return messages.length === 0 ? sessionId : null;
}

function doCreate(opts: CreateSessionOptions): Promise<string | null> {
  if (opts.cwd.trim() === "") return Promise.resolve(null);
  const owner = agentCommandOwner();
  const runtime = agentRuntime();
  const state = agentSessionState();
  const view = agentSessionView();
  const key = joinKey(opts);
  const fresh = alreadyOnAFreshSession(opts, state, view);
  if (fresh) return Promise.resolve(fresh);
  return owner
    .runSessionCreate(key, () => createAndOpen({ owner, runtime, state, ...opts }))
    .catch((error: unknown) => {
      if (owner.isCurrent()) reportSessionError("create", error);
      return null;
    });
}

/** Imperative New for non-React callers (palette commands, keymap).
 *
 * The active Session is the only authoritative source of the inherited cwd.
 * If no Session is active, or its summary has not resolved, New is a focus move
 * to the project-selection destination rather than a backend mutation.
 */
export function createSession(): Promise<string | null> {
  const sessionId = agentSessionState().getActiveSessionId();
  if (!sessionId) return Promise.resolve(null);
  const sessions = queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY]);
  const cwd = sessions?.find((session) => session.id === sessionId)?.workspace.path;
  if (!cwd || cwd.trim() === "") return Promise.resolve(null);
  return doCreate({ cwd, reuseFreshDraft: true });
}

export function useCreateSession(): (opts: CreateSessionOptions) => Promise<string | null> {
  return useCallback((opts) => doCreate(opts), []);
}

import type { SessionStatus, WorkspaceAvailability } from "@flame/runtime-contract/wire";
import { createDataQuery } from "@/plugins/sdk";
import { queryClient } from "@/lib/queryClient";
import type { AgentCommandOwner } from "../agentCommandOwner";

interface AgentSessionWorkspace {
  path: string;
  availability: WorkspaceAvailability;
}

export interface AgentSessionSummary {
  id: string;
  revision: number;
  title: string;
  status: SessionStatus;
  provider: string;
  model: string;
  reasoningEffort?: string;
  workspace: AgentSessionWorkspace;
  favorite?: boolean;
  time: string;
}

export const AGENT_SESSIONS_KEY = "sessions";

export const useAgentSessions = createDataQuery<AgentSessionSummary[]>(AGENT_SESSIONS_KEY);

export function invalidateAgentSessions(): Promise<void> {
  return queryClient.invalidateQueries({ queryKey: [AGENT_SESSIONS_KEY] });
}

// A created Session joins the projection with the revalidation that cancels
// older reads, so a list read issued before the create cannot drop it.
export function commitCreatedAgentSession(created: AgentSessionSummary): void {
  queryClient.setQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY], (sessions) =>
    sessions && !sessions.some((session) => session.id === created.id)
      ? [created, ...sessions]
      : sessions,
  );
  void invalidateAgentSessions();
}

// A queued summary write runs after the same Session's earlier writes have
// been committed to this projection, so it states the revision the Runtime
// last returned. The commit and the revalidation that cancels older reads are
// adjacent, so a read issued before the write cannot restore its predecessor.
export function writeAgentSessionSummary(
  owner: AgentCommandOwner,
  sessionId: string,
  renderedRevision: number,
  write: (expectedRevision: number) => Promise<AgentSessionSummary>,
): Promise<AgentSessionSummary> {
  return owner.serializeSessionSummary(sessionId, async () => {
    owner.assertCurrent();
    const current = queryClient
      .getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY])
      ?.find((session) => session.id === sessionId);
    const saved = await owner.settle(write(current?.revision ?? renderedRevision));
    owner.assertCurrent();
    queryClient.setQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY], (sessions) =>
      sessions?.map((session) => (session.id === saved.id ? saved : session)),
    );
    void invalidateAgentSessions();
    return saved;
  });
}

export function subscribeAgentSessionProjection<T>(
  project: (sessions: readonly AgentSessionSummary[] | undefined) => T,
  onChange: (projection: T) => void,
  equal: (left: T, right: T) => boolean = Object.is,
): () => void {
  let current = project(queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY]));
  return queryClient.getQueryCache().subscribe((event) => {
    if (event.query.queryKey.length !== 1 || event.query.queryKey[0] !== AGENT_SESSIONS_KEY) {
      return;
    }
    const next = project(event.query.state.data as AgentSessionSummary[] | undefined);
    if (equal(current, next)) return;
    current = next;
    onChange(next);
  });
}

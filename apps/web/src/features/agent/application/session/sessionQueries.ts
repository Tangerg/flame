import {
  type SessionStatus,
  type WorkspaceAvailability,
} from "@/foundation/protocolValues.generated";
import { createDataQuery } from "@/extensions/dataQuery";
import { queryClient } from "@/lib/queryClient";
import { type AgentCommandOwner } from "@/features/agent/application/agentCommandOwner";
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

// Ordering and cache publication preserve each command's captured precondition;
// a later projection cannot authorize an older edit against a newer revision.
export function writeAgentSessionSummary(
  owner: AgentCommandOwner,
  sessionId: string,
  write: () => Promise<AgentSessionSummary>,
): Promise<AgentSessionSummary> {
  return owner.serializeSessionSummary(sessionId, async () => {
    owner.assertCurrent();
    const saved = await owner.settle(write());
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

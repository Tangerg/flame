import { createDataQuery } from "@/plugins/sdk";
import { queryClient } from "@/lib/queryClient";

interface AgentSessionWorkspace {
  path: string;
  availability: "available" | "missing";
}

export interface AgentSessionSummary {
  id: string;
  revision: number;
  title: string;
  status: "running" | "waiting" | "idle";
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

export function recoverAgentSessionSummaryField(
  previous: AgentSessionSummary[] | undefined,
  sessionId: string,
  field: "title" | "favorite",
  optimisticValue: string | boolean,
): void {
  const prior = previous?.find((session) => session.id === sessionId);
  if (prior) {
    queryClient.setQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY], (current) =>
      current?.map((session) => {
        if (session.id !== sessionId || session[field] !== optimisticValue) return session;
        return { ...session, [field]: prior[field] };
      }),
    );
  }
  void invalidateAgentSessions();
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

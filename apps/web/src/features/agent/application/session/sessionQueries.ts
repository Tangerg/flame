import {
  type SessionStatus,
  type WorkspaceAvailability,
} from "@/foundation/protocolValues.generated";
import { createDataQuery } from "@/extensions/dataQuery";
import { queryClient, replaceCachedRead } from "@/lib/queryClient";
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
  projectId?: string;
  favorite?: boolean;
  time: string;
}

export const AGENT_SESSIONS_KEY = "sessions";

export const useAgentSessions = createDataQuery<AgentSessionSummary[]>(AGENT_SESSIONS_KEY);

export function invalidateAgentSessions(): Promise<void> {
  return replaceCachedRead({ queryKey: [AGENT_SESSIONS_KEY] });
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

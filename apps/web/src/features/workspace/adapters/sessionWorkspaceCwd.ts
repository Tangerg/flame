import { queryClient } from "@/lib/queryClient";
import { type AgentSessions } from "@/features/agent/public/services";
import {
  AGENT_SESSION_SUMMARY_KEY,
  fetchAgentSessionSummary,
  type AgentSessionSummary,
} from "@/features/agent/public/session";
import { ASYNC_OWNERSHIP_RETIRED, settleBeforeAbort } from "@/lib/asyncOwnership";
import {
  type WorkspaceCwdInputChange,
  type WorkspaceCwdResolution,
} from "@/features/workspace/application/workspaceEventSubscription";
export async function resolveActiveSessionWorkspaceCwd(
  sessions: Pick<AgentSessions, "getActiveSessionId">,
  signal: AbortSignal,
): Promise<WorkspaceCwdResolution> {
  const id = sessions.getActiveSessionId();
  if (!id) return { status: "resolved" };
  const summary = await settleBeforeAbort(fetchAgentSessionSummary(id), signal);
  if (!summary || summary === ASYNC_OWNERSHIP_RETIRED) return { status: "unavailable" };
  return { status: "resolved", cwd: summary.workspace.path };
}

export function subscribeWorkspaceCwdInputs(
  sessions: Pick<AgentSessions, "getActiveSessionId" | "subscribeActiveSessionId">,
  onChange: (change: WorkspaceCwdInputChange) => void,
): () => void {
  const readPath = () =>
    queryClient.getQueryData<AgentSessionSummary | null>([
      AGENT_SESSION_SUMMARY_KEY,
      sessions.getActiveSessionId(),
    ])?.workspace.path;
  let path = readPath();
  const unsubSession = sessions.subscribeActiveSessionId(() => {
    path = readPath();
    onChange("identity");
  });
  const unsubCache = queryClient.getQueryCache().subscribe(({ query }) => {
    const [key, id] = query.queryKey;
    if (key !== AGENT_SESSION_SUMMARY_KEY || id !== sessions.getActiveSessionId()) return;
    const next = readPath();
    if (path === next) return;
    path = next;
    onChange("projection");
  });
  return () => {
    unsubSession();
    unsubCache();
  };
}

import { getContainer } from "@/main/container";
import { queryClient } from "@/lib/queryClient";
import type { AgentSessions } from "@/plugins/builtin/agent/public/services";
import {
  AGENT_SESSIONS_KEY,
  subscribeAgentSessionProjection,
  type AgentSessionSummary,
} from "@/plugins/builtin/agent/public/session";
import { asSessionId, isErrorType } from "@/rpc";
import type {
  WorkspaceCwdInputChange,
  WorkspaceCwdResolution,
} from "../application/workspaceEventSubscription";

export async function resolveActiveSessionWorkspaceCwd(
  sessions: Pick<AgentSessions, "getActiveSessionId">,
  signal: AbortSignal,
): Promise<WorkspaceCwdResolution> {
  const id = sessions.getActiveSessionId();
  if (!id) return { status: "resolved" };
  const list = queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY]);
  const cached = list?.find((session) => session.id === id);
  if (cached) return { status: "resolved", cwd: cached.workspace.path };
  return getContainer()
    .client()
    .sessions.get(asSessionId(id), signal)
    .then((session) => ({ status: "resolved", cwd: session.workspace.ref.path }) as const)
    .catch((error: unknown) => {
      if (isErrorType(error, "session_not_found")) return { status: "unavailable" } as const;
      throw error;
    });
}

export function subscribeWorkspaceCwdInputs(
  sessions: Pick<AgentSessions, "getActiveSessionId" | "subscribeActiveSessionId">,
  onChange: (change: WorkspaceCwdInputChange) => void,
): () => void {
  const unsubSession = sessions.subscribeActiveSessionId(() => onChange("identity"));
  const unsubCache = subscribeAgentSessionProjection(
    (projection) => sessionWorkspaceRevision(sessions.getActiveSessionId(), projection),
    () => onChange("projection"),
  );
  return () => {
    unsubSession();
    unsubCache();
  };
}

function sessionWorkspaceRevision(
  activeSessionId: string,
  sessions: readonly AgentSessionSummary[] | undefined,
): string {
  const session = sessions?.find(({ id }) => id === activeSessionId);
  return JSON.stringify([activeSessionId, session ? session.workspace.path : null]);
}

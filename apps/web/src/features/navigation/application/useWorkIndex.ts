import { useMemo } from "react";
import { useWorkspaceProjects } from "@/features/workspace/public/queries";
import {
  resolvedWorkspaceCwd,
  useActiveSessionWorkspace,
  useActiveSessionId,
} from "@/features/agent/public/session";
import { useAgentSessions } from "@/features/agent/public/session";
import { type AgentSessionSummary } from "@/features/agent/public/session";
import { type WorkIndex } from "@/features/navigation/domain/workIndex";
import { buildWorkIndex } from "@/features/navigation/application/buildWorkIndex";
const EMPTY_SESSIONS: AgentSessionSummary[] = [];

export function useWorkIndex(): WorkIndex {
  const projects = useWorkspaceProjects();
  const { data: sessions = EMPTY_SESSIONS } = useAgentSessions();
  const activeSessionId = useActiveSessionId();
  const workspace = useActiveSessionWorkspace();
  const activeCwd = resolvedWorkspaceCwd(workspace);

  const content = useMemo(
    () => buildWorkIndex({ projects: projects.data, sessions }),
    [projects.data, sessions],
  );

  return {
    groups: content?.groups,
    recents: content?.recents,
    activeSessionId,
    activeCwd,
    isLoading: projects.isLoading && !content,
    error: projects.isError && !content ? projects.error : undefined,
    retry: () => void projects.refetch(),
  };
}

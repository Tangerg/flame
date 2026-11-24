import { useMemo } from "react";
import { useWorkspaceFolders } from "@/features/workspace/public/queries";
import { useProjects } from "@/features/projects/public/projects";
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
  const folders = useWorkspaceFolders();
  const projects = useProjects();
  const { data: sessions = EMPTY_SESSIONS } = useAgentSessions();
  const activeSessionId = useActiveSessionId();
  const workspace = useActiveSessionWorkspace();
  const activeCwd = resolvedWorkspaceCwd(workspace);

  const content = useMemo(
    () => buildWorkIndex({ folders: folders.data, projects: projects.data, sessions }),
    [folders.data, projects.data, sessions],
  );
  const groups = content?.groups;

  return {
    groups,
    projects: content?.projects,
    recents: content?.recents,
    activeSessionId,
    activeCwd,
    isLoading: folders.isPending && groups === undefined,
    error: folders.isError && groups === undefined ? folders.error : undefined,
    retry: () => void folders.refetch(),
  };
}

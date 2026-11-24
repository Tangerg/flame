import { useMemo } from "react";
import { useActiveSessionId, useAgentSessions } from "@/features/agent/public/session";
import { useProjects } from "@/features/projects/application/projectQueries";

export interface ActiveSessionProject {
  // True while the active Session names a Project whose read has not arrived, so a
  // consumer cannot fall back to the Session's folder by mistake.
  pending: boolean;
  project?: { id: string; name: string };
}

export function useActiveSessionProject(): ActiveSessionProject {
  const sessionId = useActiveSessionId();
  const { data: sessions } = useAgentSessions();
  const projects = useProjects();
  const projectId = sessions?.find((session) => session.id === sessionId)?.projectId;
  const project = projectId
    ? projects.data?.find((candidate) => candidate.id === projectId)
    : undefined;
  const id = project?.id;
  const name = project?.name;
  return useMemo(
    () => ({
      pending: Boolean(projectId) && !id,
      ...(id && name !== undefined ? { project: { id, name } } : {}),
    }),
    [projectId, id, name],
  );
}

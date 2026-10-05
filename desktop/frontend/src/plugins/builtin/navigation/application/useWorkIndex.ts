import { useMemo } from "react";
import { useWorkspaceProjects } from "@/plugins/builtin/workspace/public/queries";
import {
  useActiveSessionWorkspace,
  useActiveSessionId,
  useAgentSessions,
} from "@/plugins/builtin/agent/public/session";
import type { AgentSessionSummary } from "@/plugins/builtin/agent/public/session";
import type { WorkIndex } from "../domain/workIndex";
import { buildWorkIndex } from "./buildWorkIndex";

const EMPTY_SESSIONS: AgentSessionSummary[] = [];

export function useWorkIndex(): WorkIndex {
  const projects = useWorkspaceProjects();
  const { data: sessions = EMPTY_SESSIONS } = useAgentSessions();
  const activeSessionId = useActiveSessionId();
  const workspace = useActiveSessionWorkspace();
  const activeCwd = workspace.status === "ready" ? workspace.cwd : undefined;

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
    isError: projects.isError && !content,
    error: projects.isError && !content ? projects.error : undefined,
    retry: () => void projects.refetch(),
  };
}

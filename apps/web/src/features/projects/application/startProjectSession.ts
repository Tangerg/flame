import { useCallback } from "react";
import {
  useAgentSessions,
  useCreateSession,
  type AgentSessionSummary,
} from "@/features/agent/public/session";
import { focusComposer } from "@/features/composer/public/focus";
import { useComposer } from "@/features/composer/public/composer";
import { useWorkingDirectory } from "@/features/navigation/public/workingDirectory";
import { useWorkspace } from "@/features/workspace/public/workspace";
import { selectWorkspaceChat } from "@/features/workspace/public/navigation";

const EMPTY_SESSIONS: AgentSessionSummary[] = [];

// A new Session continues in the folder its Project last worked in; a Project
// without Sessions asks for a folder first.
export function useStartProjectSession(): (projectId: string) => void {
  const { data: sessions = EMPTY_SESSIONS } = useAgentSessions();
  const create = useCreateSession();
  const composer = useComposer();
  const directory = useWorkingDirectory();
  const workspace = useWorkspace();
  return useCallback(
    (projectId) => {
      selectWorkspaceChat(workspace);
      const latest = sessions
        .filter((session) => session.projectId === projectId)
        .reduce<AgentSessionSummary | undefined>(
          (newest, session) => (!newest || session.time > newest.time ? session : newest),
          undefined,
        );
      if (!latest) {
        directory.open(projectId);
        return;
      }
      void create({ cwd: latest.workspace.path, projectId }).then((sessionId) => {
        if (sessionId) focusComposer(composer);
      });
    },
    [composer, create, directory, sessions, workspace],
  );
}

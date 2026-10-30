import { type AgentAccess, useAgent } from "@/features/agent/public/agent";
import { type ComposerAccess, useComposer } from "@/features/composer/public/composer";
import { type RuntimeAccess, useRuntime } from "@/features/connection/public/runtime";
import {
  type WorkingDirectoryAccess,
  useWorkingDirectory,
} from "@/features/navigation/public/workingDirectory";
import { useWorkspace } from "@/features/workspace/public/workspace";
import { useMemo } from "react";
import {
  useActiveSessionId,
  resolvedWorkspaceCwd,
  useActiveSessionWorkspace,
} from "@/features/agent/public/session";
import { useCreateSession } from "@/features/agent/public/session";
import { useDeleteSession } from "@/features/agent/public/session";
import { useForkSession } from "@/features/agent/public/session";
import { useRenameSession } from "@/features/agent/public/session";
import { useToggleFavorite } from "@/features/agent/public/session";
import { focusComposer } from "@/features/composer/public/focus";
import { openSettingsView } from "@/features/workspace/public/deeplinks";
import {
  runtimeCommandsAvailable,
  useRuntimeCommandsAvailable,
} from "@/features/connection/public/serviceStatus";

export interface WorkIndexActions {
  canCreateSession: boolean;
  canCreateSessionInFolder: boolean;
  createSession: () => void;
  chooseSessionFolder: () => void;
  startSessionInFolder: (cwd: string) => void;
  selectSession: (id: string) => void;
  renameSession: (id: string, expectedRevision: number, title: string) => Promise<boolean>;
  forkSession: (id: string) => void;
  deleteSession: (id: string) => void;
  toggleFavorite: (id: string, expectedRevision: number, favorite: boolean) => void;
  openSettings: () => void;
}

export function createNewSession(
  agentAccess: AgentAccess,
  composerAccess: ComposerAccess,
  runtimeAccess: RuntimeAccess,
  directoryAccess: WorkingDirectoryAccess,
): void {
  if (!runtimeCommandsAvailable(runtimeAccess)) return;
  if (!agentAccess.getActiveSessionId()) {
    directoryAccess.open();
    return;
  }
  void agentAccess.createSession().then((sessionId) => {
    if (sessionId) focusComposer(composerAccess);
  });
}

export function useWorkIndexActions(): WorkIndexActions {
  const agentAccess = useAgent();
  const composerAccess = useComposer();
  const workspaceAccess = useWorkspace();
  const runtimeAccess = useRuntime();
  const directoryAccess = useWorkingDirectory();

  const create = useCreateSession();
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const activeSessionId = useActiveSessionId();
  const activeWorkspace = useActiveSessionWorkspace();
  const activeWorkspaceStatus = activeWorkspace.status;
  const activeCwd = resolvedWorkspaceCwd(activeWorkspace);
  const remove = useDeleteSession();
  const fork = useForkSession();
  const rename = useRenameSession();
  const toggleFavorite = useToggleFavorite();

  return useMemo(
    () => ({
      canCreateSession:
        runtimeAvailable &&
        (!activeSessionId ||
          (activeWorkspaceStatus === "ready" && Boolean(activeCwd && activeCwd.trim()))),
      canCreateSessionInFolder: runtimeAvailable,
      createSession: () => {
        if (activeSessionId && activeWorkspaceStatus !== "ready") return;
        createNewSession(agentAccess, composerAccess, runtimeAccess, directoryAccess);
      },
      chooseSessionFolder: () => {
        if (!runtimeCommandsAvailable(runtimeAccess)) return;
        directoryAccess.open();
      },
      startSessionInFolder: (cwd) => {
        if (!runtimeCommandsAvailable(runtimeAccess)) return;
        void create({ cwd }).then((sessionId) => {
          if (sessionId) focusComposer(composerAccess);
        });
      },
      selectSession: agentAccess.selectAgentSession,
      renameSession: rename,
      forkSession: (id) => {
        void fork(id);
      },
      deleteSession: (id) => {
        void remove(id);
      },
      toggleFavorite: (id, expectedRevision, favorite) => {
        void toggleFavorite(id, expectedRevision, favorite);
      },
      openSettings: () => {
        openSettingsView(workspaceAccess);
      },
    }),
    [
      activeCwd,
      activeSessionId,
      activeWorkspaceStatus,
      create,
      fork,
      remove,
      rename,
      runtimeAvailable,
      toggleFavorite,
      agentAccess,
      composerAccess,
      workspaceAccess,
      runtimeAccess,
      directoryAccess,
    ],
  );
}

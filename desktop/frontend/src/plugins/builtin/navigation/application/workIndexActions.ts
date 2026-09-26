import { useMemo } from "react";
import {
  selectAgentSession,
  createSession,
  getActiveSessionId,
  useActiveSessionId,
  useActiveSessionWorkspace,
  useCreateSession,
  useDeleteSession,
  useForkSession,
  useRenameSession,
  useToggleFavorite,
} from "@/plugins/builtin/agent/public/session";
import { focusComposer } from "@/plugins/builtin/chat/composer/public/focus";
import { showWorkspaceDock } from "@/plugins/builtin/workspace/public/navigation";
import { openSettingsView } from "@/plugins/builtin/workspace/public/deeplinks";
import {
  runtimeCommandsAvailable,
  useRuntimeCommandsAvailable,
} from "@/plugins/builtin/runtime/public/serviceStatus";
import { workingDirectoryPicker } from "./ports/workingDirectoryPicker";

export interface WorkIndexActions {
  canCreateSession: boolean;
  canCreateSessionInFolder: boolean;
  createSession: () => void;
  chooseSessionFolder: () => void;
  startSessionInFolder: (cwd: string) => void;
  selectSession: (id: string) => void;
  renameSession: (id: string, expectedRevision: number, title: string) => void;
  forkSession: (id: string) => void;
  deleteSession: (id: string) => void;
  toggleFavorite: (id: string, expectedRevision: number, favorite: boolean) => void;
  openContextDock: () => void;
  openSettings: () => void;
}

export function createNewSession(): void {
  if (!runtimeCommandsAvailable()) return;
  if (!getActiveSessionId()) {
    workingDirectoryPicker().open();
    return;
  }
  void createSession().then((sessionId) => {
    if (sessionId) focusComposer();
  });
}

export function useWorkIndexActions(): WorkIndexActions {
  const create = useCreateSession();
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const activeSessionId = useActiveSessionId();
  const activeWorkspace = useActiveSessionWorkspace();
  const activeWorkspaceStatus = activeWorkspace.status;
  const activeCwd = activeWorkspace.status === "ready" ? activeWorkspace.cwd : undefined;
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
        createNewSession();
      },
      chooseSessionFolder: () => {
        if (!runtimeCommandsAvailable()) return;
        workingDirectoryPicker().open();
      },
      startSessionInFolder: (cwd) => {
        if (!runtimeCommandsAvailable()) return;
        void create({ cwd }).then((sessionId) => {
          if (sessionId) focusComposer();
        });
      },
      selectSession: selectAgentSession,
      renameSession: (id, expectedRevision, title) => {
        void rename(id, expectedRevision, title);
      },
      forkSession: (id) => {
        void fork(id);
      },
      deleteSession: (id) => {
        void remove(id);
      },
      toggleFavorite: (id, expectedRevision, favorite) => {
        void toggleFavorite(id, expectedRevision, favorite);
      },
      openContextDock: showWorkspaceDock,
      openSettings: () => {
        openSettingsView();
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
    ],
  );
}

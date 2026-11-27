import { type FlameClient } from "@flame/runtime-client";
import { type Navigator } from "@/lib/navigation";
import { type ClientHost } from "@/platform/host";
import { type AgentAccess } from "@/features/agent/public/agent";
import { type RuntimeAccess } from "@/features/connection/public/runtime";
import { type RuntimeServerScope } from "@/features/connection/public/services";
import { type WorkspaceAccess } from "@/features/workspace/application/workspaceAccess";
import { createConversationImport } from "@/features/workspace/application/conversationImport";
import { createContextDockStore } from "@/features/workspace/adapters/contextDockStore";
import { createShellLayoutStore } from "@/features/workspace/adapters/shellLayoutStore";
import { createWorkspaceNavigation } from "@/features/workspace/adapters/navigationStatePort";
import { createLocalWorkspaceActions } from "@/features/workspace/adapters/localWorkspaceActions";
import { runtimeWorkspaceErrorClassifier } from "@/features/workspace/adapters/runtimeWorkspaceErrorClassifier";
import { bindConversationImportRuntime } from "@/features/workspace/adapters/runtimeConversationImportGateway";
import { pickTextFile } from "@/features/workspace/adapters/fileTransfer";
import { type NotificationFeed } from "@/features/notifications/public/notifications";
import { TaskWorktreeCommands } from "@/features/workspace/application/taskWorktreeCommands";
import { taskWorktreesQueryKey } from "@/features/workspace/application/workspaceQueries";
import { replaceCachedRead } from "@/lib/queryClient";
export function createWorkspace(
  client: () => FlameClient,
  navigation: Navigator,
  host: Pick<ClientHost, "openPath" | "revealPath">,
  canAccessLocalWorkspace: () => boolean,
  agent: AgentAccess,
  runtime: RuntimeAccess,
  notifications: NotificationFeed,
) {
  const dock = createContextDockStore();
  const layout = createShellLayoutStore();
  const navigationState = createWorkspaceNavigation(
    navigation,
    dock,
    layout,
    runtime.endpoint.currentRuntimeEndpoint,
  );
  const conversationImport = createConversationImport(agent, runtime, notifications, {
    pickText: pickTextFile,
  });
  const worktrees = () => client().workspaces.tasks;
  const taskWorktreeCommands = new TaskWorktreeCommands(worktrees, () => {
    void replaceCachedRead({
      queryKey: taskWorktreesQueryKey(runtime.endpoint.currentRuntimeEndpoint()),
      exact: true,
    });
  });
  const access: WorkspaceAccess = {
    navigation: navigationState,
    local: createLocalWorkspaceActions(host, canAccessLocalWorkspace),
    errors: runtimeWorkspaceErrorClassifier,
    conversationImport,
    taskWorktreeCommands,
    taskWorktrees: worktrees,
  };
  let unbindRuntime: (() => void) | null = null;
  let disposed = false;
  return {
    access,
    dock,
    layout,
    start(scope: RuntimeServerScope) {
      if (disposed) throw new Error("Workspace is disposed");
      if (unbindRuntime) return;
      dock.activate(runtime.endpoint.currentRuntimeEndpoint());
      const unbindImport = bindConversationImportRuntime(conversationImport, client, scope);
      taskWorktreeCommands.replaceTarget();
      const unsubscribe = scope.subscribeReplacement(taskWorktreeCommands.replaceTarget);
      unbindRuntime = () => {
        unsubscribe();
        taskWorktreeCommands.retireTarget();
        unbindImport();
      };
    },
    dispose() {
      if (disposed) return;
      disposed = true;
      navigationState.dispose();
      unbindRuntime?.();
      unbindRuntime = null;
      dock.store.setState({ inspection: null });
    },
  };
}
export type WorkspaceOwner = ReturnType<typeof createWorkspace>;

import { type FlameClient } from "@flame/runtime-client";
import { type Navigator } from "@/lib/navigation";
import { type ClientHost } from "@/platform/host";
import { type AgentAccess } from "@/features/agent/public/agent";
import { type RuntimeAccess } from "@/features/connection/public/runtime";
import { type WorkspaceAccess } from "@/features/workspace/application/workspaceAccess";
import { createConversationArchive } from "@/features/workspace/application/conversationExport";
import { createContextDockStore } from "@/features/workspace/adapters/contextDockStore";
import { createShellLayoutStore } from "@/features/workspace/adapters/shellLayoutStore";
import { createWorkspaceNavigation } from "@/features/workspace/adapters/navigationStatePort";
import { createLocalWorkspaceActions } from "@/features/workspace/adapters/localWorkspaceActions";
import { runtimeWorkspaceErrorClassifier } from "@/features/workspace/adapters/runtimeWorkspaceErrorClassifier";
import { createConversationArchiveGateway } from "@/features/workspace/adapters/runtimeConversationArchiveGateway";
import { createFileTransfer } from "@/features/workspace/adapters/fileTransfer";
export function createWorkspace(
  client: () => FlameClient,
  navigation: Navigator,
  host: Pick<ClientHost, "openPath" | "revealPath" | "saveFile">,
  canAccessLocalWorkspace: () => boolean,
  agent: AgentAccess,
  runtime: RuntimeAccess,
) {
  const dock = createContextDockStore();
  const layout = createShellLayoutStore();
  const navigationState = createWorkspaceNavigation(
    navigation,
    dock,
    layout,
    runtime.endpoint.currentRuntimeEndpoint,
  );
  const archive = createConversationArchive(agent, runtime);
  const access: WorkspaceAccess = {
    navigation: navigationState,
    local: createLocalWorkspaceActions(host, canAccessLocalWorkspace),
    errors: runtimeWorkspaceErrorClassifier,
    archive,
  };
  let archiveGateway: ReturnType<typeof createConversationArchiveGateway> | null = null;
  let disposed = false;
  return {
    access,
    dock,
    layout,
    start() {
      if (disposed) throw new Error("Workspace is disposed");
      if (archiveGateway) return;
      dock.activate(runtime.endpoint.currentRuntimeEndpoint());
      archiveGateway = createConversationArchiveGateway(archive, client, createFileTransfer(host));
    },
    replaceRuntimeGeneration() {
      archiveGateway?.replaceRuntimeGeneration();
    },
    dispose() {
      if (disposed) return;
      disposed = true;
      archiveGateway?.dispose();
      archiveGateway = null;
      dock.store.setState({ inspection: null });
    },
  };
}
export type WorkspaceOwner = ReturnType<typeof createWorkspace>;

import { type FlameClient } from "@flame/runtime-client";
import { type Navigator } from "@/lib/navigation";
import { type ClientHost } from "@/platform/host";
import { type AgentAccess } from "@/features/agent/public/agent";
import { type RuntimeAccess } from "@/features/connection/public/runtime";
import { type WorkspaceAccess } from "@/features/workspace/application/workspaceAccess";
import { createConversationImport } from "@/features/workspace/application/conversationImport";
import { createContextDockStore } from "@/features/workspace/adapters/contextDockStore";
import { createShellLayoutStore } from "@/features/workspace/adapters/shellLayoutStore";
import { createWorkspaceNavigation } from "@/features/workspace/adapters/navigationStatePort";
import { createLocalWorkspaceActions } from "@/features/workspace/adapters/localWorkspaceActions";
import { runtimeWorkspaceErrorClassifier } from "@/features/workspace/adapters/runtimeWorkspaceErrorClassifier";
import { createConversationImportGateway } from "@/features/workspace/adapters/runtimeConversationImportGateway";
import { pickTextFile } from "@/features/workspace/adapters/fileTransfer";
export function createWorkspace(
  client: () => FlameClient,
  navigation: Navigator,
  host: Pick<ClientHost, "openPath" | "revealPath">,
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
  const conversationImport = createConversationImport(agent, runtime);
  const access: WorkspaceAccess = {
    navigation: navigationState,
    local: createLocalWorkspaceActions(host, canAccessLocalWorkspace),
    errors: runtimeWorkspaceErrorClassifier,
    conversationImport,
  };
  let importGateway: ReturnType<typeof createConversationImportGateway> | null = null;
  let disposed = false;
  return {
    access,
    dock,
    layout,
    start() {
      if (disposed) throw new Error("Workspace is disposed");
      if (importGateway) return;
      dock.activate(runtime.endpoint.currentRuntimeEndpoint());
      importGateway = createConversationImportGateway(conversationImport, client, {
        pickText: pickTextFile,
      });
    },
    replaceRuntimeGeneration() {
      importGateway?.replaceRuntimeGeneration();
    },
    dispose() {
      if (disposed) return;
      disposed = true;
      importGateway?.dispose();
      importGateway = null;
      dock.store.setState({ inspection: null });
    },
  };
}
export type WorkspaceOwner = ReturnType<typeof createWorkspace>;

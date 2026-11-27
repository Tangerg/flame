import { type FlameClient } from "@flame/runtime-client";
import { registerWorkspaceDataProviders } from "@/features/workspace/adapters/runtimeDataProviders";
import { definePlugin } from "@/extensions/definePlugin";
import { type WorkspaceOwner } from "@/features/workspace/adapters/workspace";
import { WORKSPACE_SCOPE } from "@/features/workspace/public/services";
import { RUNTIME_SERVER_SCOPE } from "@/features/connection/public/services";
export function createWorkspaceBootstrapPlugin(
  workspace: WorkspaceOwner,
  runtimeClient: () => FlameClient,
) {
  return definePlugin({
    name: "flame.builtin.workspace-bootstrap",
    requires: { serverScope: RUNTIME_SERVER_SCOPE },
    provides: { scopes: WORKSPACE_SCOPE },
    setup(ctx) {
      registerWorkspaceDataProviders(ctx.contributions, runtimeClient);
      workspace.start(ctx.dependencies.serverScope);
      ctx.lifetime.defer(workspace.dispose);
      return {
        scopes: {
          adoptSessionScope: workspace.access.navigation.adoptSessionScope,
          activateSessionScope: workspace.access.navigation.activateSessionScope,
          forgetSessionScopes: workspace.access.navigation.forgetSessionScopes,
        },
      };
    },
  });
}

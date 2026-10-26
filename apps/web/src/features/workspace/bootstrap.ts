import { type FlameClient } from "@flame/runtime-client";
import { registerWorkspaceDataProviders } from "@/features/workspace/adapters/runtimeDataProviders";
import { definePlugin } from "@/extensions/definePlugin";
import { type WorkspaceOwner } from "@/features/workspace/adapters/workspace";
import {
  WORKSPACE_SCOPE,
  WORKSPACE_MUTATION_LIFECYCLE,
} from "@/features/workspace/public/services";
import { RUNTIME_STREAM } from "@/features/connection/public/services";
export function createWorkspaceBootstrapPlugin(
  workspace: WorkspaceOwner,
  runtimeClient: () => FlameClient,
) {
  return definePlugin({
    name: "flame.builtin.workspace-bootstrap",
    requires: { runtime: RUNTIME_STREAM },
    provides: { scopes: WORKSPACE_SCOPE, mutationLifecycle: WORKSPACE_MUTATION_LIFECYCLE },
    setup(ctx) {
      registerWorkspaceDataProviders(ctx, runtimeClient);
      workspace.start();
      ctx.cleanup(workspace.dispose);
      return {
        scopes: {
          adoptSessionScope: workspace.access.navigation.adoptSessionScope,
          activateSessionScope: workspace.access.navigation.activateSessionScope,
          forgetSessionScopes: workspace.access.navigation.forgetSessionScopes,
        },
        mutationLifecycle: { replaceRuntimeGeneration: workspace.replaceRuntimeGeneration },
      };
    },
  });
}

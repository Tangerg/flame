import { type FlameClient } from "@flame/runtime-client";
import { type ClientHost } from "@/platform/host";
import { installLocalWorkspaceActions } from "@/features/workspace/adapters/localWorkspaceActions";
import { registerWorkspaceDataProviders } from "@/features/workspace/adapters/runtimeDataProviders";
import { definePlugin } from "@/extensions/definePlugin";
import { installConversationArchiveGateway } from "@/features/workspace/adapters/runtimeConversationArchiveGateway";
import { createFileTransfer } from "@/features/workspace/adapters/fileTransfer";
import { installWorkspaceErrorClassifier } from "@/features/workspace/adapters/runtimeWorkspaceErrorClassifier";
import { installWorkspaceNavigationPort } from "@/features/workspace/adapters/navigationStatePort";
import {
  activateWorkspaceSessionScope,
  adoptWorkspaceSessionScope,
  forgetWorkspaceSessionScopes,
} from "@/features/workspace/public/navigation";
import { WORKSPACE_SCOPE } from "@/features/workspace/public/services";
import { WORKSPACE_MUTATION_LIFECYCLE } from "@/features/workspace/public/services";
import { RUNTIME_STREAM } from "@/features/connection/public/services";
import { currentRuntimeEndpoint } from "@/features/connection/public/endpoint";
export function createWorkspaceBootstrapPlugin(
  runtimeClient: () => FlameClient,
  host: Pick<ClientHost, "openPath" | "revealPath" | "saveTextFile">,
  canAccessLocalWorkspace: () => boolean,
) {
  return definePlugin({
    name: "flame.builtin.workspace-bootstrap",
    // Runtime setup installs the mutation journal before a generation captures its client.
    requires: { runtime: RUNTIME_STREAM },
    provides: {
      scopes: WORKSPACE_SCOPE,
      mutationLifecycle: WORKSPACE_MUTATION_LIFECYCLE,
    },
    setup(ctx) {
      registerWorkspaceDataProviders(ctx, runtimeClient);
      const conversationArchive = installConversationArchiveGateway(
        runtimeClient,
        createFileTransfer(host),
      );
      ctx.cleanup(() => conversationArchive.dispose());
      ctx.cleanup(installLocalWorkspaceActions(host, canAccessLocalWorkspace));
      ctx.cleanup(installWorkspaceErrorClassifier());
      ctx.cleanup(installWorkspaceNavigationPort(currentRuntimeEndpoint));
      return {
        scopes: {
          adoptSessionScope: adoptWorkspaceSessionScope,
          activateSessionScope: activateWorkspaceSessionScope,
          forgetSessionScopes: forgetWorkspaceSessionScopes,
        },
        mutationLifecycle: {
          replaceRuntimeGeneration() {
            conversationArchive.replaceRuntimeGeneration();
          },
        },
      };
    },
  });
}

import type { FlameClient } from "@flame/runtime-contract/client";
import type { ClientHost } from "@/platform/host";
import { installLocalWorkspaceActions } from "./adapters/localWorkspaceActions";
import { registerWorkspaceDataProviders } from "./adapters/runtimeDataProviders";
import { definePlugin } from "@/plugins/sdk";
import { installConversationArchiveGateway } from "./adapters/runtimeConversationArchiveGateway";
import { installAgentMemoryGateway } from "./adapters/runtimeAgentMemoryGateway";
import { installSkillCurationGateway } from "./adapters/runtimeSkillCurationGateway";
import { installWorkspaceErrorClassifier } from "./adapters/runtimeWorkspaceErrorClassifier";
import { installWorkspaceNavigationPort } from "./adapters/navigationStatePort";
import {
  activateWorkspaceSessionScope,
  adoptWorkspaceSessionScope,
  forgetWorkspaceSessionScopes,
} from "@/plugins/builtin/workspace/public/navigation";
import { WORKSPACE_SCOPE } from "@/plugins/builtin/workspace/public/services";
import { WORKSPACE_MUTATION_LIFECYCLE } from "@/plugins/builtin/workspace/public/services";
import { RUNTIME_STREAM } from "@/plugins/builtin/runtime/public/services";
import { currentRuntimeEndpoint } from "@/plugins/builtin/runtime/public/endpoint";

export function createWorkspaceBootstrapPlugin(
  runtimeClient: () => FlameClient,
  host: Pick<ClientHost, "openPath" | "revealPath">,
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
      const agentMemory = installAgentMemoryGateway(runtimeClient);
      ctx.cleanup(() => agentMemory.dispose());
      const skillCuration = installSkillCurationGateway(runtimeClient);
      ctx.cleanup(() => skillCuration.dispose());
      const conversationArchive = installConversationArchiveGateway(runtimeClient);
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
            skillCuration.replaceRuntimeGeneration();
            agentMemory.replaceRuntimeGeneration();
            conversationArchive.replaceRuntimeGeneration();
          },
        },
      };
    },
  });
}

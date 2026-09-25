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

export default definePlugin({
  name: "flame.builtin.workspace-bootstrap",
  // The owners installed here bind one Runtime connection generation — that is
  // what the mutation lifecycle replaces — and each is composed from the client
  // the container assembles out of the Runtime plugin's endpoint and mutation
  // journal. Declaring the stream is what orders this setup after that plugin;
  // without it the gateways compose against a client the container has yet to
  // finish, and then retires.
  requires: { runtime: RUNTIME_STREAM },
  provides: {
    scopes: WORKSPACE_SCOPE,
    mutationLifecycle: WORKSPACE_MUTATION_LIFECYCLE,
  },
  setup(ctx) {
    const agentMemory = installAgentMemoryGateway();
    const skillCuration = installSkillCurationGateway();
    const conversationArchive = installConversationArchiveGateway();
    const disposers = [
      () => conversationArchive.dispose(),
      () => agentMemory.dispose(),
      () => skillCuration.dispose(),
      installWorkspaceErrorClassifier(),
      installWorkspaceNavigationPort(),
    ];
    ctx.cleanup(() => {
      for (let index = disposers.length - 1; index >= 0; index--) disposers[index]!();
    });
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

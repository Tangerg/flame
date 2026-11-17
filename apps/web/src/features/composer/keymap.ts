import { type AgentAccess } from "@/features/agent/public/agent";
import { type ComposerAccess } from "@/features/composer/public/composer";
import { type RuntimeAccess } from "@/features/connection/public/runtime";

import { definePlugin } from "@/extensions/definePlugin";
import { COMPOSER_KEY_BINDING } from "@/features/composer/public/extensionPoints";
import { composerKeyBindings } from "@/features/composer/application/composerKeyBindings";
import {
  recallNextComposerHistory,
  recallPreviousComposerHistory,
} from "@/features/composer/public/history";
import { runtimeCommandsAvailable } from "@/features/connection/public/serviceStatus";
export const composerKeymap = (
  agentAccess: AgentAccess,
  composerAccess: ComposerAccess,
  runtimeAccess: RuntimeAccess,
) =>
  definePlugin({
    name: "flame.builtin.composer-keymap",
    setup(ctx) {
      for (const binding of composerKeyBindings({
        send: ({ submit }) => {
          if (!runtimeCommandsAvailable(runtimeAccess)) return true;
          submit();
          return true;
        },
        declineApproval: () =>
          runtimeCommandsAvailable(runtimeAccess)
            ? agentAccess.submitPendingApproval("deny")
            : true,
        stopRun: () => runtimeCommandsAvailable(runtimeAccess) && agentAccess.stopCurrentRootRun(),
        historyPrevious: () => recallPreviousComposerHistory(composerAccess),
        historyNext: () => recallNextComposerHistory(composerAccess),
      })) {
        ctx.contributions.add(COMPOSER_KEY_BINDING, binding);
      }
    },
  });

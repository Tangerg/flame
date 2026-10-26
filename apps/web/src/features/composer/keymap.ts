import { type AgentAccess } from "@/features/agent/public/agent";
import { type ComposerAccess } from "@/features/composer/public/composer";
import { type RuntimeAccess } from "@/features/connection/public/runtime";
import { submitPendingApproval } from "@/features/agent/public/hitl";
import { stopCurrentRootRun } from "@/features/agent/public/run";
import { definePlugin } from "@/extensions/definePlugin";
import { COMPOSER_KEY_BINDING } from "@/features/composer/public/extensionPoints";
import { composerKeyBindings } from "@/features/composer/application/composerKeyBindings";
import {
  recallNextHistoryFromKey,
  recallPreviousHistoryFromKey,
} from "@/features/composer/application/composerHistoryKeys";
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
        approveOrSend: ({ submit }) => {
          if (!runtimeCommandsAvailable(runtimeAccess)) return true;
          if (submitPendingApproval(agentAccess, "approve")) return true;
          submit();
          return true;
        },
        declineApproval: () =>
          runtimeCommandsAvailable(runtimeAccess)
            ? submitPendingApproval(agentAccess, "deny")
            : true,
        stopRun: () => runtimeCommandsAvailable(runtimeAccess) && stopCurrentRootRun(agentAccess),
        historyPrevious: ({ event }) =>
          recallPreviousHistoryFromKey({
            event,
            recall: recallPreviousComposerHistory.bind(null, composerAccess),
          }),
        historyNext: ({ event }) =>
          recallNextHistoryFromKey({
            event,
            recall: recallNextComposerHistory.bind(null, composerAccess),
          }),
      })) {
        ctx.contribute(COMPOSER_KEY_BINDING, binding);
      }
    },
  });

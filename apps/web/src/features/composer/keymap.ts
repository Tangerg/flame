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
export const composerKeymap = definePlugin({
  name: "flame.builtin.composer-keymap",
  setup(ctx) {
    for (const binding of composerKeyBindings({
      send: ({ submit }) => {
        if (!runtimeCommandsAvailable()) return true;
        submit();
        return true;
      },
      approveOrSend: ({ submit }) => {
        if (!runtimeCommandsAvailable()) return true;
        if (submitPendingApproval("approve")) return true;
        submit();
        return true;
      },
      declineApproval: () => (runtimeCommandsAvailable() ? submitPendingApproval("deny") : true),
      stopRun: () => runtimeCommandsAvailable() && stopCurrentRootRun(),
      historyPrevious: ({ event }) =>
        recallPreviousHistoryFromKey({ event, recall: recallPreviousComposerHistory }),
      historyNext: ({ event }) =>
        recallNextHistoryFromKey({ event, recall: recallNextComposerHistory }),
    })) {
      ctx.contribute(COMPOSER_KEY_BINDING, binding);
    }
  },
});

import { contributeLayout, definePlugin, notifyError, type SlashCommandSpec } from "@/plugins/sdk";
import {
  COMPOSER_SUBMIT_MODE,
  SLASH_COMMAND,
  TOOL_STANDING_SURFACE,
} from "@/plugins/sdk/kernelPoints";
import { t } from "@/lib/i18n";
import { rpcErrorText } from "@/lib/rpcErrors";
import { installGoalRuntimeAdapter } from "./adapters/runtimeGoalCommandsGateway";
import { GoalStatusSurface } from "./ui/GoalStatusSurface";
import { GoalModeIndicator } from "./ui/GoalModeIndicator";
import { RUNTIME_STREAM, followRuntimeGeneration } from "@/plugins/builtin/runtime/public/services";
import { getActiveSessionId } from "@/plugins/builtin/agent/public/session";
import { getAgentSessionSharedMaterial } from "@/plugins/builtin/agent/public/sessionMaterial";
import { getComposerText } from "@/plugins/builtin/chat/composer/public/draft";
import { focusComposer } from "@/plugins/builtin/chat/composer/public/focus";
import { selectedComposerModelPreference } from "@/plugins/builtin/chat/composer/public/modelPreference";
import { runtimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";
import { goalCommandWasRetired, startGoal } from "./application/goalCommands";
import { GoalComposerModeOwner } from "./application/goalComposerMode";
import { createGoalComposerSubmitMode } from "./application/goalComposerSubmitMode";
import type { GoalState } from "./application/goalReadModel";

const GOAL_SURFACE = "composer.overlay.top:goal";

const GOAL_SLASH_COMMAND: SlashCommandSpec = {
  description: "slash.goal",
};

export default definePlugin({
  name: "flame.builtin.goal",
  requires: { runtime: RUNTIME_STREAM },
  setup(ctx) {
    const composerMode = GoalComposerModeOwner.install();
    const runtimeAdapter = installGoalRuntimeAdapter(ctx.runtime.connectionGeneration() !== null);
    // Retired generation stops its commands; a REPLACED one re-arms them.
    const unsubscribeRuntime = followRuntimeGeneration(ctx.runtime, (next) => {
      if (next === null) runtimeAdapter.retireRuntimeGeneration();
      else runtimeAdapter.replaceRuntimeGeneration();
    });
    contributeLayout(ctx, "composer.overlay.top", {
      id: "goal",
      order: 10,
      component: GoalStatusSurface,
    });
    contributeLayout(ctx, "composer.toolbar.start", {
      id: "goal-mode",
      order: 4,
      component: GoalModeIndicator,
    });
    for (const key of ["create_goal", "get_goal"]) {
      ctx.contribute(TOOL_STANDING_SURFACE, GOAL_SURFACE, { key });
    }
    ctx.contribute(
      COMPOSER_SUBMIT_MODE,
      createGoalComposerSubmitMode(composerMode, {
        getActiveSessionId,
        composerText: getComposerText,
        goalState: (sessionId) => getAgentSessionSharedMaterial<GoalState>(sessionId, "goal"),
        runtimeAvailable: runtimeCommandsAvailable,
        modelPreference: selectedComposerModelPreference,
        start: startGoal,
        focusComposer,
        reportUnavailable: () => notifyError(t("goal.error.unavailable")),
        reportUnsupportedAttachments: () => notifyError(t("goal.error.attachmentsUnsupported")),
        reportStartError: (error) => notifyError(rpcErrorText(error) ?? t("goal.error.start")),
        retired: goalCommandWasRetired,
      }),
    );
    ctx.contribute(SLASH_COMMAND, GOAL_SLASH_COMMAND, { key: "/goal" });
    ctx.cleanup(() => {
      unsubscribeRuntime();
      composerMode.dispose();
      runtimeAdapter.dispose();
    });
  },
});

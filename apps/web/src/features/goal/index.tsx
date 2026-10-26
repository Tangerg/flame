import { type AgentAccess } from "@/features/agent/public/agent";
import { type ComposerAccess } from "@/features/composer/public/composer";
import { type RuntimeAccess } from "@/features/connection/public/runtime";
import { toolFamilyNames } from "@/lib/toolFamilies";
import { type FlameClient } from "@flame/runtime-client";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { notifyError } from "@/features/notifications/public/notifications";
import { resolveAgentRunStartOptions } from "@/features/agent/public/runOptions";
import { type SlashCommandSpec } from "@/features/composer/public/types";
import { COMPOSER_SUBMIT_MODE, SLASH_COMMAND } from "@/features/composer/public/extensionPoints";
import { TOOL_STANDING_SURFACE } from "@/features/chat/public/tools/types";
import { t } from "@/lib/i18n/index";
import { rpcErrorText } from "@/lib/rpcErrors";
import { installGoalRuntimeAdapter } from "@/features/goal/adapters/runtimeGoalCommandsGateway";
import { GoalStatusSurface } from "@/features/goal/ui/GoalStatusSurface";
import { GoalModeIndicator } from "@/features/goal/ui/GoalModeIndicator";
import { RUNTIME_STREAM, followRuntimeGeneration } from "@/features/connection/public/services";

import { focusComposer } from "@/features/composer/public/focus";
import { runtimeCommandsAvailable } from "@/features/connection/public/serviceStatus";
import { startGoal } from "@/features/goal/application/goalCommands";
import { GoalComposerModeOwner } from "@/features/goal/application/goalComposerMode";
import { createGoalComposerSubmitMode } from "@/features/goal/application/goalComposerSubmitMode";
import { type GoalState } from "@/features/goal/application/goalReadModel";
const GOAL_SURFACE = "composer.overlay.top:goal";

const GOAL_SLASH_COMMAND: SlashCommandSpec = {
  description: "slash.goal",
};

export function createGoalPlugin(
  agentAccess: AgentAccess,
  composerAccess: ComposerAccess,
  runtimeAccess: RuntimeAccess,
  runtimeClient: () => FlameClient,
) {
  return definePlugin({
    name: "flame.builtin.goal",
    requires: { runtime: RUNTIME_STREAM },
    setup(ctx) {
      const composerMode = GoalComposerModeOwner.install();
      ctx.cleanup(() => composerMode.dispose());
      const runtimeAdapter = installGoalRuntimeAdapter(
        agentAccess,
        runtimeAccess,
        runtimeClient,
        ctx.runtime.connectionGeneration() !== null,
      );
      ctx.cleanup(() => runtimeAdapter.dispose());
      ctx.cleanup(
        followRuntimeGeneration(ctx.runtime, (next) => {
          if (next === null) runtimeAdapter.retireRuntimeGeneration();
          else runtimeAdapter.replaceRuntimeGeneration();
        }),
      );
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
      for (const key of toolFamilyNames("goals")) {
        ctx.contribute(TOOL_STANDING_SURFACE, GOAL_SURFACE, { key });
      }
      ctx.contribute(
        COMPOSER_SUBMIT_MODE,
        createGoalComposerSubmitMode(composerMode, {
          getActiveSessionId: agentAccess.getActiveSessionId,
          goalState: (sessionId) =>
            agentAccess.getAgentSessionSharedMaterial<GoalState>(sessionId, "goal"),
          runtimeAvailable: runtimeCommandsAvailable.bind(null, runtimeAccess),
          runOptions: resolveAgentRunStartOptions,
          start: startGoal,
          focusComposer: focusComposer.bind(null, composerAccess),
          reportUnavailable: () => notifyError(t("goal.error.unavailable")),
          reportUnsupportedAttachments: () => notifyError(t("goal.error.attachmentsUnsupported")),
          reportStartError: (error) => notifyError(rpcErrorText(error) ?? t("goal.error.start")),
          retired: wasGenerationRetired,
        }),
      );
      ctx.contribute(SLASH_COMMAND, GOAL_SLASH_COMMAND, { key: "/goal" });
    },
  });
}

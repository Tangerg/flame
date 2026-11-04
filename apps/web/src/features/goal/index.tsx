import { GOAL_WORKFLOW, type WorkflowProviders } from "@/features/plugins/public/workflows";
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
import { createGoalRuntimeAdapter } from "@/features/goal/adapters/runtimeGoalCommandsGateway";
import { GoalStatusSurface } from "@/features/goal/ui/GoalStatusSurface";
import { GoalModeIndicator } from "@/features/goal/ui/GoalModeIndicator";
import { RUNTIME_STREAM, followRuntimeGeneration } from "@/features/connection/public/services";

import { focusComposer } from "@/features/composer/public/focus";
import { runtimeCommandsAvailable } from "@/features/connection/public/serviceStatus";
import { type Goals } from "@/features/goal/application/goals";
import { GenerationRetiredError } from "@/lib/asyncOwnership";
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
  goals: Goals,
  providers: WorkflowProviders,
) {
  return definePlugin({
    name: "flame.builtin.goal",
    requires: { runtime: RUNTIME_STREAM },
    setup(ctx) {
      const composerMode = goals.composerMode;
      ctx.lifetime.defer(() => composerMode.reset());
      const runtimeAdapter = createGoalRuntimeAdapter(
        agentAccess,
        runtimeAccess,
        runtimeClient,
        goals.commands,
        ctx.dependencies.runtime.connectionGeneration() !== null,
      );
      ctx.lifetime.defer(() => runtimeAdapter.dispose());
      ctx.lifetime.defer(
        followRuntimeGeneration(ctx.dependencies.runtime, (next) => {
          composerMode.reset();
          if (next === null) runtimeAdapter.retireRuntimeGeneration();
          else runtimeAdapter.replaceRuntimeGeneration();
        }),
      );
      contributeLayout(ctx.contributions, "composer.overlay.top", {
        id: "goal",
        order: 10,
        component: GoalStatusSurface,
      });
      contributeLayout(ctx.contributions, "composer.toolbar.start", {
        id: "goal-mode",
        order: 4,
        component: GoalModeIndicator,
      });
      for (const key of toolFamilyNames("goals")) {
        ctx.contributions.add(TOOL_STANDING_SURFACE, GOAL_SURFACE, { key });
      }
      // A withdrawn /goal draft must never be delivered as an ordinary chat message.
      ctx.contributions.add(
        COMPOSER_SUBMIT_MODE,
        createGoalComposerSubmitMode(composerMode, {
          getActiveSessionId: agentAccess.getActiveSessionId,
          goalState: (sessionId) => {
            const state = agentAccess.getAgentSessionSharedMaterial<GoalState>(sessionId, "goal");
            if (state?.goal && !providers.admits(GOAL_WORKFLOW, state.goal.author))
              return { ...state, available: false };
            return state;
          },
          runtimeAvailable: () =>
            providers.selected(GOAL_WORKFLOW) !== undefined &&
            runtimeCommandsAvailable(runtimeAccess),
          runOptions: resolveAgentRunStartOptions,
          captureStart: () => {
            const source = providers.selected(GOAL_WORKFLOW);
            const commands = goals.commands.capture();
            return (input) => {
              const original = agentAccess.getAgentSessionSharedMaterial<GoalState>(
                input.sessionId,
                "goal",
              )?.goal;
              if (
                !commands ||
                source === undefined ||
                providers.selected(GOAL_WORKFLOW) !== source ||
                (original && !providers.admits(GOAL_WORKFLOW, original.author))
              )
                throw new GenerationRetiredError("goal_command_generation");
              return commands.start(input);
            };
          },
          focusComposer: focusComposer.bind(null, composerAccess),
          reportUnavailable: () => notifyError(t("goal.error.unavailable")),
          reportUnsupportedAttachments: () => notifyError(t("goal.error.attachmentsUnsupported")),
          reportStartError: (error) => notifyError(rpcErrorText(error) ?? t("goal.error.start")),
          retired: wasGenerationRetired,
        }),
      );
      let previous: string | undefined;
      let unregister: (() => void) | undefined;
      const refresh = () => {
        const source = providers.selected(GOAL_WORKFLOW);
        if (source === previous) return;
        previous = source;
        unregister?.();
        unregister = undefined;
        composerMode.reset();
        if (source === undefined) return;
        const slash = ctx.contributions.add(SLASH_COMMAND, GOAL_SLASH_COMMAND, { key: "/goal" });
        unregister = () => slash[Symbol.dispose]();
      };
      ctx.lifetime.defer(providers.subscribe(refresh));
      refresh();
    },
  });
}

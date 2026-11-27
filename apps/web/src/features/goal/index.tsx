import { disposeSymbol } from "dougong";
import { GOAL_WORKFLOW, type WorkflowProviders } from "@/features/plugins/public/workflows";
import { type AgentAccess } from "@/features/agent/public/agent";
import { type ComposerAccess } from "@/features/composer/public/composer";
import { type RuntimeAccess } from "@/features/connection/public/runtime";
import { toolFamilyNames } from "@/features/agent/public/toolFamilies";
import { type FlameClient } from "@flame/runtime-client";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { type NotificationFeed } from "@/features/notifications/public/notifications";
import { resolveAgentRunStartOptions } from "@/features/agent/public/runOptions";
import { SLASH_COMMAND } from "@/features/composer/public/extensionPoints";
import { TOOL_STANDING_SURFACE } from "@/features/chat/public/tools/types";
import { t } from "@/lib/i18n/index";
import { rpcErrorText } from "@/lib/rpcErrors";
import { bindGoalRuntime } from "@/features/goal/adapters/runtimeGoalCommandsGateway";
import { GoalStatusSurface } from "@/features/goal/ui/GoalStatusSurface";
import { GoalModeIndicator } from "@/features/goal/ui/GoalModeIndicator";
import { RUNTIME_SERVER_SCOPE } from "@/features/connection/public/services";

import { focusComposer } from "@/features/composer/public/focus";
import { runtimeCommandsAvailable } from "@/features/connection/public/serviceStatus";
import { type Goals } from "@/features/goal/application/goals";
import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { createGoalComposerCommand } from "@/features/goal/application/goalComposerCommand";
import {
  GOAL_MATERIAL,
  GOAL_SLASH_COMMAND,
  type GoalState,
} from "@/features/goal/application/goalReadModel";
import { GOAL_COMMAND_SCOPE } from "@/features/goal/application/goalCommands";
const GOAL_SURFACE = "composer.overlay.top:goal";

export function createGoalPlugin(
  agentAccess: AgentAccess,
  composerAccess: ComposerAccess,
  runtimeAccess: RuntimeAccess,
  runtimeClient: () => FlameClient,
  goals: Goals,
  providers: WorkflowProviders,
  notifications: NotificationFeed,
) {
  return definePlugin({
    name: "flame.builtin.goal",
    requires: { scope: RUNTIME_SERVER_SCOPE },
    setup(ctx) {
      const submissions = goals.submissions;
      ctx.lifetime.defer(() => submissions.reset());
      ctx.lifetime.defer(
        bindGoalRuntime(
          agentAccess,
          runtimeAccess,
          runtimeClient,
          goals.commands,
          ctx.dependencies.scope,
        ),
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
      const run = createGoalComposerCommand(submissions, {
        getActiveSessionId: agentAccess.getActiveSessionId,
        goalState: (sessionId) => {
          const state = agentAccess.getAgentSessionSharedMaterial<GoalState>(
            sessionId,
            GOAL_MATERIAL,
          );
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
              GOAL_MATERIAL,
            )?.goal;
            if (
              !commands ||
              source === undefined ||
              providers.selected(GOAL_WORKFLOW) !== source ||
              (original && !providers.admits(GOAL_WORKFLOW, original.author))
            )
              throw new GenerationRetiredError(GOAL_COMMAND_SCOPE);
            return commands.start(input);
          };
        },
        prepareObjective: () => {
          composerAccess.editLeadingSlashCommand(GOAL_SLASH_COMMAND, "prepareArguments");
          focusComposer(composerAccess);
        },
        reportUnavailable: () => notifications.notify("error", t("goal.error.unavailable")),
        reportUnsupportedAttachments: () =>
          notifications.notify("error", t("goal.error.attachmentsUnsupported")),
        reportStartError: (error) =>
          notifications.notify("error", rpcErrorText(error) ?? t("goal.error.start")),
        retired: wasGenerationRetired,
      });
      let previous: boolean | undefined;
      let unregister: (() => void) | undefined;
      const refresh = () => {
        const available = providers.selected(GOAL_WORKFLOW) !== undefined;
        if (available === previous) return;
        previous = available;
        unregister?.();
        // Saved Goal drafts still resolve this command while its provider is unavailable.
        const slash = ctx.contributions.add(
          SLASH_COMMAND,
          { description: available ? "slash.goal" : undefined, run },
          { key: GOAL_SLASH_COMMAND },
        );
        unregister = () => slash[disposeSymbol]();
      };
      ctx.lifetime.defer(providers.subscribe(refresh));
      refresh();
    },
  });
}

import { type AgentAccess } from "@/features/agent/public/agent";
import { type RuntimeAccess } from "@/features/connection/public/runtime";
import { type FlameClient } from "@flame/runtime-client";
import { definePlugin } from "@/extensions/definePlugin";
import { AGENT_SESSIONS } from "@/features/agent/public/services";
import { installProjectIndexRefresh } from "@/features/workspace/adapters/projectIndexRefresh";
import {
  invalidateWorkspaceEvent,
  invalidateWorkspaceEverything,
  replaceWorkspaceServerScope,
  retireWorkspaceReadModels,
} from "@/features/workspace/adapters/queryInvalidation";
import {
  canSubscribeWorkspaceEvents,
  subscribeRuntimeWorkspaceEvents,
} from "@/features/workspace/adapters/runtimeWorkspaceEvents";
import {
  resolveActiveSessionWorkspaceCwd,
  subscribeWorkspaceCwdInputs,
} from "@/features/workspace/adapters/sessionWorkspaceCwd";
import { createWorkspaceEventLoop } from "@/features/workspace/application/workspaceEventLoop";
import { startWorkspaceEventSubscription } from "@/features/workspace/application/workspaceEventSubscription";
import {
  installWorkspaceFocusRefresh,
  subscribeWorkspaceReadTargets,
  workspaceReadTargets,
} from "@/features/workspace/adapters/workspaceReadObservation";
import { RUNTIME_SERVER_SCOPE, RUNTIME_STREAM } from "@/features/connection/public/services";
import { WORKSPACE_MUTATION_LIFECYCLE } from "@/features/workspace/public/services";
export function createWorkspaceEventsPlugin(
  agentAccess: AgentAccess,
  runtimeAccess: RuntimeAccess,
  runtimeClient: () => FlameClient,
) {
  return definePlugin({
    name: "flame.builtin.workspace-events",
    requires: {
      runtime: RUNTIME_STREAM,
      serverScope: RUNTIME_SERVER_SCOPE,
      mutationLifecycle: WORKSPACE_MUTATION_LIFECYCLE,
      sessions: AGENT_SESSIONS,
    },
    setup(ctx) {
      const loop = createWorkspaceEventLoop({
        subscribe: ({ target, signal }) =>
          subscribeRuntimeWorkspaceEvents(runtimeAccess, runtimeClient, target, signal),
        handleEvent: invalidateWorkspaceEvent.bind(null, agentAccess),
        invalidateAll: invalidateWorkspaceEverything.bind(null, agentAccess),
        reportDisconnect: (connectionGeneration) => {
          ctx.dependencies.runtime.reportConnectionLoss(connectionGeneration);
        },
      });

      ctx.lifetime.defer(installProjectIndexRefresh());
      ctx.lifetime.defer(installWorkspaceFocusRefresh());
      ctx.lifetime.defer(
        ctx.dependencies.serverScope.subscribeReplacement(
          replaceWorkspaceServerScope.bind(null, agentAccess),
        ),
      );
      ctx.lifetime.defer(
        startWorkspaceEventSubscription({
          canSubscribe: canSubscribeWorkspaceEvents.bind(null, runtimeAccess),
          connectionGeneration: ctx.dependencies.runtime.connectionGeneration,
          subscribeConnection: ctx.dependencies.runtime.subscribeConnection,
          retireReadModels: () => {
            ctx.dependencies.mutationLifecycle.replaceRuntimeGeneration();
            retireWorkspaceReadModels(agentAccess);
          },
          resolveWorkspaceCwd: (signal) =>
            resolveActiveSessionWorkspaceCwd(runtimeClient, ctx.dependencies.sessions, signal),
          reportResolutionError: (error) =>
            console.warn("[workspace-events] target resolution failed:", error),
          subscribeWorkspaceCwdInputs: (onChange) =>
            subscribeWorkspaceCwdInputs(ctx.dependencies.sessions, onChange),
          readTargets: workspaceReadTargets,
          subscribeReadTargets: subscribeWorkspaceReadTargets,
          loop,
        }),
      );
    },
  });
}

import type { FlameClient } from "@flame/runtime-contract/client";
import { definePlugin } from "@/plugins/sdk";
import { AGENT_SESSIONS } from "@/plugins/builtin/agent/public/services";
import { installProjectIndexRefresh } from "./adapters/projectIndexRefresh";
import {
  invalidateWorkspaceEvent,
  invalidateWorkspaceEverything,
  replaceWorkspaceServerScope,
  retireWorkspaceReadModels,
} from "./adapters/queryInvalidation";
import {
  canSubscribeWorkspaceEvents,
  subscribeRuntimeWorkspaceEvents,
} from "./adapters/runtimeWorkspaceEvents";
import {
  resolveActiveSessionWorkspaceCwd,
  subscribeWorkspaceCwdInputs,
} from "./adapters/sessionWorkspaceCwd";
import { createWorkspaceEventLoop } from "./application/workspaceEventLoop";
import { startWorkspaceEventSubscription } from "./application/workspaceEventSubscription";
import {
  installWorkspaceFocusRefresh,
  subscribeWorkspaceReadTargets,
  workspaceReadTargets,
} from "./adapters/workspaceReadObservation";
import { RUNTIME_SERVER_SCOPE, RUNTIME_STREAM } from "@/plugins/builtin/runtime/public/services";
import { WORKSPACE_MUTATION_LIFECYCLE } from "@/plugins/builtin/workspace/public/services";

export function createWorkspaceEventsPlugin(runtimeClient: () => FlameClient) {
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
          subscribeRuntimeWorkspaceEvents(runtimeClient, target, signal),
        handleEvent: invalidateWorkspaceEvent,
        invalidateAll: invalidateWorkspaceEverything,
        reportDisconnect: (connectionGeneration) => {
          ctx.runtime.reportConnectionLoss(connectionGeneration);
        },
      });

      ctx.cleanup(installProjectIndexRefresh());
      ctx.cleanup(installWorkspaceFocusRefresh());
      ctx.cleanup(ctx.serverScope.subscribeReplacement(replaceWorkspaceServerScope));
      ctx.cleanup(
        startWorkspaceEventSubscription({
          canSubscribe: canSubscribeWorkspaceEvents,
          connectionGeneration: ctx.runtime.connectionGeneration,
          subscribeConnection: ctx.runtime.subscribeConnection,
          retireReadModels: () => {
            ctx.mutationLifecycle.replaceRuntimeGeneration();
            retireWorkspaceReadModels();
          },
          resolveWorkspaceCwd: (signal) =>
            resolveActiveSessionWorkspaceCwd(runtimeClient, ctx.sessions, signal),
          reportResolutionError: (error) =>
            console.warn("[workspace-events] target resolution failed:", error),
          subscribeWorkspaceCwdInputs: (onChange) =>
            subscribeWorkspaceCwdInputs(ctx.sessions, onChange),
          readTargets: workspaceReadTargets,
          subscribeReadTargets: subscribeWorkspaceReadTargets,
          loop,
        }),
      );
    },
  });
}

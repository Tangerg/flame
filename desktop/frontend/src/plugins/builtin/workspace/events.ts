// Built-in plugin: the app's ONE runtime.subscribe consumer.
//
// The plugin entry is now only the composition root. Runtime subscription,
// active-cwd resolution, reconnect/retarget looping, and query invalidation
// live in their owning layers under this bounded context.

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
import { RUNTIME_SERVER_SCOPE, RUNTIME_STREAM } from "@/plugins/builtin/runtime/public/services";
import { WORKSPACE_MUTATION_LIFECYCLE } from "@/plugins/builtin/workspace/public/services";

export default definePlugin({
  name: "flame.builtin.workspace-events",
  requires: {
    runtime: RUNTIME_STREAM,
    serverScope: RUNTIME_SERVER_SCOPE,
    mutationLifecycle: WORKSPACE_MUTATION_LIFECYCLE,
    sessions: AGENT_SESSIONS,
  },
  setup(ctx) {
    const loop = createWorkspaceEventLoop({
      subscribe: ({ target, signal }) => subscribeRuntimeWorkspaceEvents(target, signal),
      handleEvent: invalidateWorkspaceEvent,
      invalidateAll: invalidateWorkspaceEverything,
      reportDisconnect: (connectionGeneration) => {
        void ctx.runtime.reportConnectionLoss(connectionGeneration);
      },
    });

    const disposeProjectIndex = installProjectIndexRefresh();
    const disposeServerScope = ctx.serverScope.subscribeReplacement(replaceWorkspaceServerScope);
    const disposeSubscription = startWorkspaceEventSubscription({
      canSubscribe: canSubscribeWorkspaceEvents,
      connectionGeneration: ctx.runtime.connectionGeneration,
      subscribeConnection: ctx.runtime.subscribeConnection,
      retireReadModels: () => {
        ctx.mutationLifecycle.replaceRuntimeGeneration();
        retireWorkspaceReadModels();
      },
      resolveWorkspaceCwd: (signal) => resolveActiveSessionWorkspaceCwd(ctx.sessions, signal),
      reportResolutionError: (error) =>
        console.warn("[workspace-events] target resolution failed:", error),
      subscribeWorkspaceCwdInputs: (onChange) =>
        subscribeWorkspaceCwdInputs(ctx.sessions, onChange),
      loop,
    });

    ctx.cleanup(() => {
      disposeSubscription();
      disposeServerScope();
      disposeProjectIndex();
    });
  },
});

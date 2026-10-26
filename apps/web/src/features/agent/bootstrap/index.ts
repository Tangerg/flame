import { registerAgentDataProviders } from "@/features/agent/adapters/runtimeDataProviders";
import { type FlameClient } from "@flame/runtime-client";
import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { PendingRunOpenings } from "@/features/agent/ui/PendingRunOpenings";
import { type AgentOwner } from "@/features/agent/adapters/agent";
import { AGENT_SESSIONS } from "@/features/agent/public/services";
import { RUNTIME_STREAM, RUNTIME_SERVER_SCOPE } from "@/features/connection/public/services";
export function createAgentBootstrapPlugin(agent: AgentOwner, runtimeClient: () => FlameClient) {
  return definePlugin({
    name: "flame.builtin.agent-bootstrap",
    requires: { runtime: RUNTIME_STREAM, scope: RUNTIME_SERVER_SCOPE },
    provides: { sessions: AGENT_SESSIONS },
    setup(ctx) {
      contributeLayout(ctx, "chat.banner.top", {
        id: "pending-run-openings",
        order: 0,
        component: PendingRunOpenings,
      });
      registerAgentDataProviders(ctx, runtimeClient);
      agent.start();
      ctx.cleanup(agent.dispose);
      return {
        sessions: {
          getActiveSessionId: agent.access.state.getActiveSessionId,
          getLifecycleSnapshot: agent.access.state.getLifecycleSnapshot,
          subscribeActiveSessionId: agent.access.state.subscribeActiveSessionId,
          subscribeLifecycle: agent.access.state.subscribeLifecycle,
          subscribeDeleted: agent.access.state.subscribeDeletedSession,
        },
      };
    },
  });
}

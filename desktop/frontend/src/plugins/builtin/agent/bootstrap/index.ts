import { definePlugin } from "@/plugins/sdk";
import { installAbandonedDraftCleanup } from "../adapters/abandonedDraftCleanup";
import { installAgentDefaultSessionPort } from "../adapters/agentDefaultSessionPort";
import { installAgentRuntimeGateway } from "../adapters/agentRuntimeGateway";
import { installAgentStatePorts } from "../adapters/agentStatePorts";
import { contributeRuntimePendingWork } from "../adapters/runtimePendingWorkProvider";
import { installInterruptResponseCoordinator } from "../application/hitl/interruptResponseCoordinator";
import {
  getActiveSessionId,
  getAgentSessionLifecycleSnapshot,
  subscribeActiveSessionId,
  subscribeAgentSessionLifecycle,
} from "@/plugins/builtin/agent/public/session";
import { AGENT_SESSIONS } from "@/plugins/builtin/agent/public/services";
import { RUNTIME_STREAM, followRuntimeGeneration } from "@/plugins/builtin/runtime/public/services";

export default definePlugin({
  name: "flame.builtin.agent-bootstrap",
  requires: { runtime: RUNTIME_STREAM },
  provides: { sessions: AGENT_SESSIONS },
  setup(ctx) {
    contributeRuntimePendingWork(ctx);
    const disposeState = installAgentStatePorts();
    const disposeDefaultSession = installAgentDefaultSessionPort();
    const runtimeGateway = installAgentRuntimeGateway();
    const unsubscribeRuntime = followRuntimeGeneration(ctx.runtime, () =>
      runtimeGateway.replaceRuntimeGeneration(),
    );
    const disposeInterruptResponses = installInterruptResponseCoordinator();
    // After the ports it reads through.
    const disposeDraftCleanup = installAbandonedDraftCleanup();
    ctx.cleanup(() => {
      disposeDraftCleanup();
      disposeInterruptResponses();
      unsubscribeRuntime();
      runtimeGateway.dispose();
      disposeDefaultSession();
      disposeState();
    });
    return {
      sessions: {
        getActiveSessionId,
        getLifecycleSnapshot: getAgentSessionLifecycleSnapshot,
        subscribeActiveSessionId,
        subscribeLifecycle: subscribeAgentSessionLifecycle,
      },
    };
  },
});

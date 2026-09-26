import { definePlugin } from "@/plugins/sdk";
import { installAgentSessionScope } from "../adapters/agentSessionScope";
import { installAgentDefaultSessionPort } from "../adapters/agentDefaultSessionPort";
import { installAgentRuntimeGateway } from "../adapters/agentRuntimeGateway";
import { installAgentStatePorts } from "../adapters/agentStatePorts";
import { installInterruptResponseCoordinator } from "../application/hitl/interruptResponseCoordinator";
import {
  getActiveSessionId,
  getAgentSessionLifecycleSnapshot,
  subscribeActiveSessionId,
  subscribeAgentSessionLifecycle,
} from "@/plugins/builtin/agent/public/session";
import { AGENT_SESSIONS } from "@/plugins/builtin/agent/public/services";
import {
  RUNTIME_SERVER_SCOPE,
  RUNTIME_STREAM,
  followRuntimeGeneration,
} from "@/plugins/builtin/runtime/public/services";
import { currentRuntimeEndpoint } from "@/plugins/builtin/runtime/public/endpoint";

export default definePlugin({
  name: "flame.builtin.agent-bootstrap",
  requires: { runtime: RUNTIME_STREAM, scope: RUNTIME_SERVER_SCOPE },
  provides: { sessions: AGENT_SESSIONS },
  setup(ctx) {
    const disposeState = installAgentStatePorts();
    const disposeDefaultSession = installAgentDefaultSessionPort();
    const runtimeGateway = installAgentRuntimeGateway();
    const unsubscribeRuntime = followRuntimeGeneration(ctx.runtime, () =>
      runtimeGateway.replaceRuntimeGeneration(),
    );
    const disposeInterruptResponses = installInterruptResponseCoordinator();
    const disposeSessionScope = installAgentSessionScope(ctx.scope, currentRuntimeEndpoint);
    ctx.cleanup(() => {
      disposeSessionScope();
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

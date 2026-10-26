import { type FlameClient } from "@flame/runtime-client";
import { type Navigator } from "@/lib/navigation";
import { type RuntimeAccess } from "@/features/connection/public/runtime";
import {
  type RuntimeStream,
  type RuntimeServerScope,
  followRuntimeGeneration,
} from "@/features/connection/public/services";
import { type AgentAccess } from "@/features/agent/application/agentAccess";
import { createAgentCommands } from "@/features/agent/application/agentCommandOwner";
import { createAgentSessionUsage } from "@/features/agent/application/session/sessionUsage";
import { createAgentSharedMaterial } from "@/features/agent/application/session/sharedMaterial";
import { InterruptResponseCoordinator } from "@/features/agent/application/hitl/interruptResponseCoordinator";
import { createAgentSessionStore } from "@/features/agent/adapters/agentSessionStore";
import { createAgentStore } from "@/features/agent/adapters/agentStore";
import { createAgentState } from "@/features/agent/adapters/agentStatePorts";
import { createAgentRuntimeGateway } from "@/features/agent/adapters/agentRuntimeGateway";
import { createAgentDefaultSession } from "@/features/agent/adapters/agentDefaultSessionPort";
import { installAgentSessionScope } from "@/features/agent/adapters/agentSessionScope";
export function createAgent(
  client: () => FlameClient,
  navigation: Navigator,
  runtime: RuntimeAccess,
  stream: RuntimeStream,
  scope: RuntimeServerScope,
) {
  const sessions = createAgentSessionStore();
  const projection = createAgentStore(sessions);
  const state = createAgentState(navigation, sessions, projection);
  const commands = createAgentCommands();
  const usage = createAgentSessionUsage();
  const sharedMaterial = createAgentSharedMaterial();
  const gateway = createAgentRuntimeGateway(client, runtime, sharedMaterial, commands, usage);
  const interrupts = new InterruptResponseCoordinator(state.view);
  const defaultSession = createAgentDefaultSession(
    navigation,
    () => ({ agentAccess: access, projection, sessions, retries: gateway.retries }),
    client,
    () => {
      const generation = stream.connectionGeneration();
      if (generation) stream.reportConnectionLoss(generation);
    },
  );
  const access: AgentAccess = {
    state: state.state,
    view: state.view,
    defaultSession,
    runtime: gateway.gateway,
    commands,
    usage,
    sharedMaterial,
    interrupts,
  };
  const stops: Array<() => void> = [];
  let started = false;
  let disposed = false;
  return {
    access,
    sessions,
    projection,
    retries: gateway.retries,
    replaceRuntimeGeneration: gateway.replaceRuntimeGeneration,
    start() {
      if (disposed) throw new Error("Agent is disposed");
      if (started) return;
      started = true;
      stops.push(
        installAgentSessionScope(
          navigation,
          sessions,
          scope,
          runtime.endpoint.currentRuntimeEndpoint,
        ),
      );
      stops.push(followRuntimeGeneration(stream, () => gateway.replaceRuntimeGeneration()));
    },
    dispose() {
      if (disposed) return;
      disposed = true;
      for (const stop of stops.splice(0).reverse()) stop();
      interrupts.retire();
      sharedMaterial.dispose();
      gateway.dispose();
      state.dispose();
      projection.dispose();
    },
  };
}
export type AgentOwner = ReturnType<typeof createAgent>;

import { createElement, type ReactNode } from "react";
import { AgentScope } from "@/features/agent/application/agentContext";
import { createAgentAccess } from "@/features/agent/application/agentAccess";
import { type FlameClient } from "@flame/runtime-client";
import { type Navigator } from "@/lib/navigation";
import { type RuntimeAccess } from "@/features/connection/public/runtime";
import {
  type RuntimeStream,
  type RuntimeServerScope,
  followRuntimeGeneration,
} from "@/features/connection/public/services";
import { type AgentDependencies } from "@/features/agent/application/agentDependencies";
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
import { type NotificationFeed } from "@/features/notifications/public/notifications";
export function createAgent(
  client: () => FlameClient,
  navigation: Navigator,
  runtime: RuntimeAccess,
  stream: RuntimeStream,
  scope: RuntimeServerScope,
  notifications: NotificationFeed,
) {
  const { access, Provider, start, dispose } = composeAgent(
    client,
    navigation,
    runtime,
    stream,
    scope,
    notifications,
  );
  return { access, Provider, start, dispose };
}
export type AgentOwner = ReturnType<typeof createAgent>;

// The UI lab replaces dependencies of this one composition; createAgent exposes only the owner surface.
export function composeAgent(
  client: () => FlameClient,
  navigation: Navigator,
  runtime: RuntimeAccess,
  stream: RuntimeStream,
  scope: RuntimeServerScope,
  notifications: NotificationFeed,
) {
  const sessions = createAgentSessionStore();
  const projection = createAgentStore(sessions);
  const state = createAgentState(navigation, sessions, projection);
  const commands = createAgentCommands();
  const usage = createAgentSessionUsage();
  const sharedMaterial = createAgentSharedMaterial();
  const gateway = createAgentRuntimeGateway(
    client,
    runtime.capabilities,
    sharedMaterial,
    commands,
    usage,
  );
  const interrupts = new InterruptResponseCoordinator(state.view);
  const defaultSession = createAgentDefaultSession(
    navigation,
    () => ({ agentAccess: dependencies, projection, sessions, retries: gateway.retries }),
    client,
    () => {
      const generation = stream.connectionGeneration();
      if (generation) stream.reportConnectionLoss(generation);
    },
  );
  let dependencies: AgentDependencies = {
    notifications,
    state: state.state,
    view: state.view,
    defaultSession,
    runtime: gateway.gateway,
    commands,
    usage,
    sharedMaterial,
    interrupts,
  };
  let access = createAgentAccess(dependencies);
  const stops: Array<() => void> = [];
  let started = false;
  let disposed = false;
  return {
    get access() {
      return access;
    },
    get dependencies() {
      return dependencies;
    },
    set dependencies(next: AgentDependencies) {
      dependencies = next;
      access = createAgentAccess(next);
    },
    sessions,
    projection,
    Provider({ children }: { children: ReactNode }) {
      return createElement(AgentScope, { dependencies, access }, children);
    },
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

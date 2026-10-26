import { onTestFinished } from "vitest";
import { type FlameClient } from "@flame/runtime-client";
import { createAgentRuntimeGateway } from "@/features/agent/adapters/agentRuntimeGateway";
import { createAgentState } from "@/features/agent/adapters/agentStatePorts";
import { InterruptResponseCoordinator } from "@/features/agent/application/hitl/interruptResponseCoordinator";
import { type TestClient } from "@/test/renderClient";

/** Recompose an adapter against this test's retained view and generation owners. */
export function createAgentGatewayFixture(client: TestClient, runtimeClient: () => FlameClient) {
  const access = client.agent.dependencies;
  const gateway = createAgentRuntimeGateway(
    runtimeClient,
    client.runtime,
    access.sharedMaterial,
    access.commands,
    access.usage,
  );
  client.agent.dependencies = { ...access, runtime: gateway.gateway };
  client.agent.retries = gateway.retries;
  onTestFinished(gateway.dispose);
  return gateway;
}

export function createAgentStateFixture(client: TestClient) {
  const state = createAgentState(client.navigation, client.agent.sessions, client.agent.projection);
  client.agent.dependencies = {
    ...client.agent.dependencies,
    state: state.state,
    view: state.view,
  };
  onTestFinished(state.dispose);
  return state;
}

export function createInterruptResponseFixture(client: TestClient) {
  client.agent.dependencies.interrupts.retire();
  const coordinator = new InterruptResponseCoordinator(client.agent.dependencies.view);
  client.agent.dependencies = { ...client.agent.dependencies, interrupts: coordinator };
  const dispose = () => coordinator.retire();
  onTestFinished(dispose);
  return dispose;
}

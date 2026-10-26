import { onTestFinished } from "vitest";
import { type FlameClient } from "@flame/runtime-client";
import { createAgentRuntimeGateway } from "@/features/agent/adapters/agentRuntimeGateway";
import { createAgentState } from "@/features/agent/adapters/agentStatePorts";
import { InterruptResponseCoordinator } from "@/features/agent/application/hitl/interruptResponseCoordinator";
import { type TestClient } from "@/test/renderClient";

/** Recompose an adapter against this test's retained view and generation owners. */
export function createAgentGatewayFixture(client: TestClient, runtimeClient: () => FlameClient) {
  const access = client.agent.access;
  const gateway = createAgentRuntimeGateway(
    runtimeClient,
    client.runtime,
    access.sharedMaterial,
    access.commands,
    access.usage,
  );
  client.agent.access = { ...access, runtime: gateway.gateway };
  client.agent.retries = gateway.retries;
  onTestFinished(gateway.dispose);
  return gateway;
}

export function createAgentStateFixture(client: TestClient) {
  const state = createAgentState(client.navigation, client.agent.sessions, client.agent.projection);
  client.agent.access = { ...client.agent.access, state: state.state, view: state.view };
  onTestFinished(state.dispose);
  return state;
}

export function createInterruptResponseFixture(client: TestClient) {
  client.agent.access.interrupts.retire();
  const coordinator = new InterruptResponseCoordinator(client.agent.access.view);
  client.agent.access = { ...client.agent.access, interrupts: coordinator };
  const dispose = () => coordinator.retire();
  onTestFinished(dispose);
  return dispose;
}

import type { FlameClient } from "@flame/runtime-client";
import { composeAgent } from "@/features/agent/adapters/agent";
import { createRuntimeConnectionProjection } from "@/features/connection/adapters/runtimeConnectionProjection";
import { createRuntimeEndpoint } from "@/features/connection/application/runtimeEndpoint";
import { NotificationFeed } from "@/features/notifications/public/notifications";
import { createMemoryNavigator } from "@/lib/navigation.testkit";

export function openWebSession(
  client: FlameClient,
  endpoint: string,
  sessionId: string,
  present: ConstructorParameters<typeof NotificationFeed>[0] = () => {},
) {
  const connection = createRuntimeConnectionProjection();
  const target = () => ({ endpoint });
  const runtime = {
    capabilities: connection.capabilities,
    status: connection.status,
    endpoint: createRuntimeEndpoint({
      read: target,
      defaultTarget: target,
      replace: () => false,
      reset: () => false,
    }),
  };
  const notifications = new NotificationFeed(present);
  const agent = composeAgent(
    () => client,
    createMemoryNavigator({ session: sessionId }),
    runtime,
    connection.stream,
    connection.scope,
    notifications,
  );
  if (sessionId) {
    agent.sessions.store.getState().holdOpen(sessionId);
    agent.projection.store.getState().ensureSession(sessionId);
  }
  return {
    agent,
    connection,
    notifications,
    runtime,
    dispose() {
      agent.dispose();
      notifications.dispose();
      connection.dispose();
    },
  };
}

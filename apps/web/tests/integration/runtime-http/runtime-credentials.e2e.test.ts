import { asSessionId } from "@flame/runtime-client";
import { expect, it } from "vitest";
import { runtimeRequestMeta } from "@/app/runtimeProtocol";
import { startKernel, stopKernel } from "@/extensions/bootstrap";
import { definePlugin } from "@/extensions/definePlugin";
import { createAgent } from "@/features/agent/adapters/agent";
import { createRuntimeConnection } from "@/features/connection/adapters/runtimeConnection";
import { bindGoalRuntime } from "@/features/goal/adapters/runtimeGoalCommandsGateway";
import { createGoals } from "@/features/goal/application/goals";
import { NotificationFeed } from "@/features/notifications/public/notifications";
import { bindConversationImportRuntime } from "@/features/workspace/adapters/runtimeConversationImportGateway";
import { createConversationImport } from "@/features/workspace/application/conversationImport";
import { createMemoryNavigator } from "@/lib/navigation.testkit";
import { createBrowserHost } from "@/platform/browserHost";
import { createProviderGate } from "./fakeProvider";
import { useRuntimeFixture } from "./fixture";
import { MemoryMutationStorage } from "./mutationStorage";

const fixture = useRuntimeFixture("runtime-credentials", {
  packages: ["goals", "memory", "providers", "schedules"],
  withLocalToken: true,
});

it("dispatches both commands after the desktop host supplies its local credential", async () => {
  const endpoint = fixture.baseUrl;
  const localToken = fixture.localToken;
  if (!localToken) throw new Error("the Runtime did not create its local credential");
  const workspace = { path: fixture.root };
  const source = await fixture.client.sessions.create({
    workspace,
    title: "Imported after desktop startup",
  });
  const sourceId = asSessionId(source.id);
  const { artifact } = await fixture.client.sessions.export(sourceId, "json");
  if (!artifact) throw new Error("the Runtime did not export a Session artifact");
  await fixture.client.sessions.delete(sourceId);
  const goalSession = await fixture.client.sessions.create({ workspace });
  const goalSessionId = asSessionId(goalSession.id);
  const gate = createProviderGate("E2E_GOAL_AFTER_CREDENTIAL");
  fixture.providerGate = gate;

  const credentialArrives = Promise.withResolvers<void>();
  let firstBootstrap = true;
  const connection = createRuntimeConnection(
    {
      ...createBrowserHost(),
      kind: "desktop",
      async bootstrap() {
        if (firstBootstrap) {
          firstBootstrap = false;
          return { runtime: { endpoint }, localFilesystemEndpoint: endpoint };
        }
        await credentialArrives.promise;
        return { runtime: { endpoint, localToken }, localFilesystemEndpoint: endpoint };
      },
    },
    runtimeRequestMeta,
    new MemoryMutationStorage(),
  );
  const notices: string[] = [];
  const notifications = new NotificationFeed(({ level }) => notices.push(level));
  const agent = createAgent(
    connection.client,
    createMemoryNavigator(),
    connection.access,
    connection.stream,
    connection.scope,
    notifications,
  );
  const imports = createConversationImport(agent.access, connection.access, notifications, {
    pickText: async () => JSON.stringify(artifact),
  });
  const goals = createGoals();
  const preferences = new Map<string, unknown>();
  let kernel: Awaited<ReturnType<typeof startKernel>> | undefined;
  try {
    await connection.initialize();
    kernel = await startKernel([
      definePlugin({
        name: "flame.test.desktop-runtime-credential",
        setup({ lifetime }) {
          const binding = connection.bind({
            lifetime,
            storage: {
              get: preferences.get.bind(preferences),
              set: preferences.set.bind(preferences),
              remove: preferences.delete.bind(preferences),
              clear: preferences.clear.bind(preferences),
              keys: () => [...preferences.keys()],
            },
          });
          lifetime.defer(binding.dispose);
          lifetime.defer(
            bindConversationImportRuntime(imports, connection.client, connection.scope),
          );
          lifetime.defer(
            bindGoalRuntime(agent.access, connection.client, goals.commands, connection.scope),
          );
        },
      }),
    ]);
    expect(connection.access.status.snapshot().phase).toBe("checking");
    credentialArrives.resolve();
    await connection.access.status.refresh();
    expect(connection.access.status.snapshot().phase).toBe("ready");
    const supported = connection.access.capabilities.hasCapability;
    expect(supported("sessionExport")).toBe(true);
    expect(supported("goals")).toBe(true);

    const goalCommands = goals.commands.capture();
    if (!goalCommands) throw new Error("the Goal command target was not bound");
    const objective = "E2E_GOAL_AFTER_CREDENTIAL verify desktop startup.";
    const [importOutcome, goalOutcome] = await Promise.allSettled([
      imports.importConversationJson(),
      goalCommands.start({ sessionId: goalSessionId, objective }),
    ]);
    expect.soft(importOutcome).toEqual({ status: "fulfilled", value: undefined });
    expect.soft(notices).toEqual(["success"]);
    expect.soft(agent.access.getActiveSessionId()).toBe(sourceId);
    expect.soft(goalOutcome).toEqual({ status: "fulfilled", value: undefined });

    await expect(fixture.client.sessions.get(sourceId)).resolves.toMatchObject({
      id: sourceId,
      title: source.title,
    });
    await expect(fixture.client.goals.get(goalSessionId)).resolves.toMatchObject({
      sessionId: goalSessionId,
      objective,
      status: "active",
    });
  } finally {
    credentialArrives.resolve();
    gate.release.resolve();
    fixture.providerGate = undefined;
    if (kernel) await stopKernel(kernel);
    goals.dispose();
    agent.dispose();
    notifications.dispose();
    await connection.dispose();
    const goal = await fixture.client.goals.get(goalSessionId);
    if (goal) await fixture.client.goals.clear(goal);
    await fixture.client.sessions.delete(goalSessionId);
    const imported = (await fixture.client.sessions.list().autoPagingToArray()).some(
      ({ id }) => id === sourceId,
    );
    if (imported) await fixture.client.sessions.delete(sourceId);
  }
}, 30_000);

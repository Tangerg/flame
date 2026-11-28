import {
  asRunId,
  asSessionId,
  createFlameClient,
  createHttpTransport,
} from "@flame/runtime-client";
import { HTTP_HEADERS, HTTP_ENDPOINTS } from "@flame/runtime-client/wire";
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
import { collectRunEvents, originalOpeningEvents } from "./observation";
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

it("reconnects Web and Desktop to the same registered Runtime and original approval", async () => {
  const endpoint = fixture.baseUrl;
  const token = fixture.localToken;
  if (!token) throw new Error("the test Runtime has no owner token");
  const webPreferences = new Map<string, unknown>();
  const desktopPreferences = new Map<string, unknown>();
  const connect = async (kind: "web" | "desktop", preferences: Map<string, unknown>) => {
    const connection = createRuntimeConnection(
      {
        ...createBrowserHost(),
        kind,
        bootstrap: async () => ({
          runtime: { endpoint: "http://127.0.0.1:1" },
          localFilesystemEndpoint: null,
        }),
      },
      runtimeRequestMeta,
      new MemoryMutationStorage(),
    );
    await connection.initialize();
    const finalizers: (() => Promise<void>)[] = [];
    const binding = connection.bind({
      lifetime: {
        defer(close) {
          const dispose = async () => {
            await close();
          };
          finalizers.push(dispose);
          return { [Symbol.asyncDispose]: dispose };
        },
      },
      storage: {
        get: preferences.get.bind(preferences),
        set: preferences.set.bind(preferences),
        remove: preferences.delete.bind(preferences),
        clear: preferences.clear.bind(preferences),
        keys: () => [...preferences.keys()],
      },
    });
    let closed = false;
    return {
      ...connection,
      async close() {
        if (closed) return;
        closed = true;
        binding.dispose();
        for (const close of finalizers) await close();
        await connection.dispose();
      },
    };
  };

  const web = await connect("web", webPreferences);
  const desktop = await connect("desktop", desktopPreferences);
  try {
    const registered = await web.access.endpoint.trustRuntime(
      endpoint,
      "Shared Runtime",
      "",
      token,
    );
    expect(registered.kind).toBe("applied");
    const store = web.access.endpoint.trustedRuntime()?.store;
    if (!store) throw new Error("registration did not capture the Runtime store");
    expect(JSON.stringify([...webPreferences])).not.toContain(token);
    const preflight = await fetch(endpoint + HTTP_ENDPOINTS.rpc.path, {
      method: "OPTIONS",
      headers: {
        Origin: "wails://localhost",
        "Access-Control-Request-Method": "POST",
        "Access-Control-Request-Headers": HTTP_HEADERS.runtimeStore,
      },
    });
    expect(preflight.headers.get("Access-Control-Allow-Headers")?.toLowerCase()).toContain(
      HTTP_HEADERS.runtimeStore.toLowerCase(),
    );
    const rejected = await desktop.access.endpoint.trustRuntime(
      endpoint,
      "Wrong store",
      "other-store",
      token,
    );
    expect(rejected).toMatchObject({ kind: "rejected", reason: "trust_failed" });
    expect(desktop.access.endpoint.trustedRuntime()).toBeUndefined();
    const attempted = createFlameClient(
      createHttpTransport({
        baseUrl: endpoint,
        localToken: token,
        expectedRuntimeStore: "other-store",
      }),
    );
    try {
      await expect(attempted.sessions.list()).rejects.toThrow();
    } finally {
      await attempted.close();
    }
    await web.access.status.refresh();
    const session = await web
      .client()
      .sessions.create({ workspace: { path: fixture.root }, title: "Original remote task" });
    const start = await web.client().runs.start({
      sessionId: asSessionId(session.id),
      input: [{ type: "text", text: "E2E_APPROVAL run the command after approval." }],
    });
    const runId = asRunId(start.result.runId);
    const events = await collectRunEvents(originalOpeningEvents(start));
    expect(events.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      run: { status: "waiting" },
    });
    const original = (await web.client().interrupts.list({ rootRunId: runId })).data[0]
      ?.interrupts[0];
    if (original?.type !== "approval")
      throw new Error("Runtime did not retain an original approval");
    await web.close();
    expect(
      (await desktop.access.endpoint.trustRuntime(endpoint, "Shared Runtime", store, token)).kind,
    ).toBe("applied");
    await desktop.access.status.refresh();
    const saved = await desktop.client().sessions.get(asSessionId(session.id));
    expect(saved.workspace.ref.path).toBe(session.workspace.ref.path);
    expect(
      (await desktop.client().interrupts.list({ rootRunId: runId })).data[0]?.interrupts[0],
    ).toEqual(original);
    const resumed = await desktop.client().runs.resume({
      runId,
      responses: [{ itemId: original.itemId, response: { type: "approval", decision: "approve" } }],
    });
    await collectRunEvents(originalOpeningEvents(resumed));
    const browserAgain = await connect("web", webPreferences);
    try {
      expect(browserAgain.access.endpoint.applyRuntimeEndpoint(endpoint, token).kind).toBe(
        "applied",
      );
      await expect(
        browserAgain.client().interrupts.list({ rootRunId: runId }),
      ).resolves.toMatchObject({ data: [] });
      await expect(browserAgain.client().runs.get(runId)).resolves.toMatchObject({
        id: runId,
        status: "finished",
      });
    } finally {
      await browserAgain.close();
    }

    const reopened = await connect("desktop", desktopPreferences);
    try {
      expect(reopened.access.endpoint.trustedRuntime()).toEqual({
        endpoint,
        name: "Shared Runtime",
        store,
      });
      expect(reopened.access.endpoint.hasRuntimeAccessToken()).toBe(false);
      expect(reopened.access.endpoint.applyRuntimeEndpoint(endpoint, token).kind).toBe("applied");
      const retained = await reopened.client().sessions.get(asSessionId(session.id));
      expect(retained.id).toBe(session.id);
    } finally {
      await reopened.close();
    }
  } finally {
    await desktop.close();
    await web.close();
  }
}, 35_000);

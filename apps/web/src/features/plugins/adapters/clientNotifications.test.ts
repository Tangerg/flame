import { asyncDisposeSymbol } from "dougong";
import { expect, it, onTestFinished, vi } from "vitest";
import { type RootRunSettlement } from "@flame/component-sdk";
import { definePlugin, type ContributionLifetime } from "@/extensions/definePlugin";
import { startKernel, stopKernel } from "@/extensions/bootstrap";
import { usePluginErrorStore } from "@/extensions/errors";
import { type AgentAccess } from "@/features/agent/public/agent";
import { createBrowserHost } from "@/platform/browserHost";
import { createTestClient } from "@/test/renderClient";
import {
  createComponentNotifications,
  createComponentSettlementSubscription,
} from "./clientNotifications";

async function fixture() {
  let lifetime!: ContributionLifetime;
  const host = await startKernel([
    definePlugin({
      name: "test.component-notifications",
      setup(scope) {
        lifetime = scope.lifetime("notifications");
      },
    }),
  ]);
  onTestFinished(() => stopKernel(host));
  const controller = new AbortController();
  const signal = AbortSignal.any([controller.signal, lifetime.signal]);
  const component = {
    installationId: "installed",
    digest: "a".repeat(64),
    componentId: "notifications",
  };
  const platform = {
    authorization: vi.fn(async () => "granted" as const),
    requestAuthorization: vi.fn(async () => "granted" as const),
    send: vi.fn(async (_notification: unknown) => true),
  };
  const client = createTestClient({
    host: {
      ...createBrowserHost(),
      notificationAuthorization: platform.authorization,
      requestNotificationAuthorization: platform.requestAuthorization,
      sendNotification: platform.send,
    },
  });
  onTestFinished(client.dispose);
  const notifier = client.notifications.access.notifier;
  const channel = createComponentNotifications(
    lifetime,
    signal,
    component,
    client.notifications.access,
  );
  return { lifetime, controller, signal, component, platform, notifier, channel };
}

it("projects the platform's permission owner and stops observing it on withdrawal", async () => {
  const { channel, controller, platform } = await fixture();
  const changed = vi.fn();
  const stop = channel.subscribe(changed);
  expect(channel.applicationName).toBe("Flame");
  expect(channel.authorization()).toBe("default");
  await expect(channel.refreshAuthorization()).resolves.toBe("granted");
  expect(channel.authorization()).toBe("granted");
  expect(changed).toHaveBeenCalledOnce();
  controller.abort();
  stop();
  expect(() => channel.requestAuthorization()).toThrow(controller.signal.reason);
  expect(platform.requestAuthorization).not.toHaveBeenCalled();
});

it("joins an accepted permission request but refuses a send after its source retires", async () => {
  const { channel, platform, notifier, lifetime, signal } = await fixture();
  const permission = Promise.withResolvers<"granted">();
  platform.requestAuthorization.mockReturnValueOnce(permission.promise);
  const requested = channel.requestAuthorization();
  const outcome = requested.then(
    (value) => ({ kind: "value", value }),
    (error) => ({ kind: "error", error }),
  );
  let closed = false;
  const closing = lifetime[asyncDisposeSymbol]().then(() => {
    closed = true;
  });
  try {
    expect(signal.aborted).toBe(true);
    expect(closed).toBe(false);
    expect(() =>
      channel.send({ id: "completion", sessionId: "captured", title: "Finished", body: "Body" }),
    ).toThrow(signal.reason);
    expect(platform.send).not.toHaveBeenCalled();
  } finally {
    permission.resolve("granted");
    await closing;
  }
  expect(await outcome).toEqual({ kind: "error", error: signal.reason });
  expect(notifier.useNotificationAuthorization.getState().authorization).toBe("granted");
});

it("captures a namespaced notification and preserves a platform send failure", async () => {
  const { channel, platform, signal } = await fixture();
  const request = {
    id: "run:captured",
    sessionId: "captured",
    title: "Original",
    body: "Original body",
  };
  const failure = new Error("notification service unavailable");
  platform.send.mockRejectedValueOnce(failure);
  const sending = channel.send(request);
  request.title = "Later";
  request.sessionId = "later";
  await expect(sending).rejects.toBe(failure);
  expect(platform.send).toHaveBeenCalledExactlyOnceWith({
    id: JSON.stringify(["installed", "notifications", "run:captured"]),
    title: "Original",
    body: "Original body",
    target: "captured",
  });
  expect(signal.aborted).toBe(false);
});

it("copies settled root facts, joins callbacks and refuses a late producer after unsubscription", async () => {
  const { lifetime, signal, component } = await fixture();
  let emit!: Parameters<AgentAccess["subscribeRootRunSettlements"]>[0];
  const released = vi.fn();
  const subscribe = createComponentSettlementSubscription(
    lifetime,
    signal,
    component,
    (listener) => {
      emit = listener;
      return released;
    },
  );
  const completed = Promise.withResolvers<void>();
  const captured: RootRunSettlement[] = [];
  const stop = subscribe((settlement) => {
    captured.push(settlement);
    return completed.promise;
  });
  const fact = {
    sessionId: "captured",
    runId: "run-fixture",
    segmentId: "segment-fixture",
    status: "finished" as const,
    errorMessage: null,
  };
  emit(fact);
  fact.sessionId = "later";
  expect(captured[0]!.sessionId).toBe("captured");
  expect(Object.isFrozen(captured[0])).toBe(true);
  stop();
  emit(fact);
  expect(captured).toHaveLength(1);
  expect(released).toHaveBeenCalledOnce();
  let closed = false;
  const closing = lifetime[asyncDisposeSymbol]().then(() => {
    closed = true;
  });
  try {
    await Promise.resolve();
    expect(closed).toBe(false);
  } finally {
    completed.resolve();
    await closing;
  }
});

it("keeps a failing plugin observer separate from the fact producer", async () => {
  const { lifetime, signal, component } = await fixture();
  usePluginErrorStore.getState().clearAll();
  let emit!: Parameters<AgentAccess["subscribeRootRunSettlements"]>[0];
  const subscribe = createComponentSettlementSubscription(
    lifetime,
    signal,
    component,
    (listener) => {
      emit = listener;
      return () => {};
    },
  );
  const failure = new Error("notification channel failed");
  subscribe(() => {
    throw failure;
  });
  expect(() =>
    emit({
      sessionId: "captured",
      runId: "run-fixture",
      segmentId: "segment-fixture",
      status: "error",
      errorMessage: "run failed",
    }),
  ).not.toThrow();
  await vi.waitFor(() =>
    expect(usePluginErrorStore.getState().log).toContainEqual(
      expect.objectContaining({ source: "events", message: failure.message }),
    ),
  );
});

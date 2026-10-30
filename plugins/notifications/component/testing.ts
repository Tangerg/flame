import { vi, onTestFinished } from "vitest";
import type {
  ComponentContext,
  NotificationAuthorization,
  RootRunSettlement,
  SystemNotificationChannel,
} from "@flame/component-sdk";
import type { FlameClient } from "@flame/runtime-client";

export function notificationFixture() {
  const controller = new AbortController();
  const tasks = new Set<Promise<unknown>>();
  const permissions = new Set<() => void>();
  const settlements = new Set<
    Parameters<ComponentContext["subscribeRootRunSettlements"]>[0]
  >();
  let authorization: NotificationAuthorization = "granted";
  const channel: SystemNotificationChannel = {
    applicationName: "Flame",
    authorization: () => authorization,
    subscribe(listener) {
      permissions.add(listener);
      return () => {
        permissions.delete(listener);
      };
    },
    refreshAuthorization: vi.fn(async () => authorization),
    requestAuthorization: vi.fn(async () => authorization),
    send: vi.fn(async () => {
      controller.signal.throwIfAborted();
      return true;
    }),
  };
  const context: ComponentContext = {
    component: {
      installationId: "installed",
      digest: "a".repeat(64),
      componentId: "notifications",
    },
    locale: "en",
    signal: controller.signal,
    runtime: {} as FlameClient,
    telemetry: {
      read(): never {
        throw new Error("Notifications must not read telemetry");
      },
      clear(): never {
        throw new Error("Notifications must not clear telemetry");
      },
    },
    track(operation) {
      controller.signal.throwIfAborted();
      const work = operation(controller.signal);
      tasks.add(work);
      void work.then(
        () => tasks.delete(work),
        () => tasks.delete(work),
      );
      return work;
    },
    notifications: channel,
    subscribeRootRunSettlements(listener) {
      controller.signal.throwIfAborted();
      settlements.add(listener);
      const stop = () => {
        settlements.delete(listener);
        controller.signal.removeEventListener("abort", stop);
      };
      controller.signal.addEventListener("abort", stop, { once: true });
      return stop;
    },
    registerCommand(): never {
      throw new Error("Notifications must not register commands");
    },
    saveFile(): never {
      throw new Error("Notifications must not save files");
    },
    compose(): never {
      throw new Error("Notifications must not compose a view");
    },
  };
  const dispose = async () => {
    controller.abort();
    await Promise.allSettled(tasks);
  };
  onTestFinished(dispose);
  return {
    context,
    channel,
    controller,
    settlements,
    setAuthorization(value: NotificationAuthorization) {
      authorization = value;
      for (const listener of permissions) listener();
    },
    settle(event: RootRunSettlement) {
      return Promise.all(
        [...settlements].map((listener) => listener(event, controller.signal)),
      );
    },
  };
}

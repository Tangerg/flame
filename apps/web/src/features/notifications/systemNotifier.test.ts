import { expect, it, vi } from "vitest";
import { createSystemNotifier } from "@/features/notifications/systemNotifier";
import { type NotificationCentre } from "@/features/notifications/application/notificationCentre";

function centre(overrides: Partial<NotificationCentre> = {}): NotificationCentre {
  return {
    authorization: async () => "default",
    requestAuthorization: async () => "granted",
    send: async () => true,
    onOpened: () => () => undefined,
    ...overrides,
  };
}

it("keeps a retired authorization result out of both the retired and successor instances", async () => {
  const pending = Promise.withResolvers<"denied">();
  const retired = createSystemNotifier(centre({ authorization: () => pending.promise }));
  const successor = createSystemNotifier(centre({ authorization: async () => "granted" }));
  const late = retired.refreshNotificationAuthorization();
  retired.dispose();
  await successor.refreshNotificationAuthorization();
  pending.resolve("denied");
  await late;
  expect(retired.useNotificationAuthorization.getState().authorization).toBe("default");
  expect(successor.useNotificationAuthorization.getState().authorization).toBe("granted");
  successor.dispose();
});

it("does not let an earlier platform read overwrite the latest authorization request", async () => {
  const pending = Promise.withResolvers<"denied">();
  const notifier = createSystemNotifier(centre({ authorization: () => pending.promise }));
  const earlier = notifier.refreshNotificationAuthorization();
  await notifier.requestNotificationAuthorization();
  pending.resolve("denied");
  await earlier;
  expect(notifier.useNotificationAuthorization.getState().authorization).toBe("granted");
  notifier.dispose();
});

it("releases opened subscriptions and refuses delivery after disposal", async () => {
  const stop = vi.fn();
  let deliver: ((target: string) => void) | undefined;
  const send = vi.fn(async () => true);
  const notifier = createSystemNotifier(
    centre({
      send,
      onOpened(open) {
        deliver = open;
        return stop;
      },
    }),
  );
  const opened = vi.fn();
  const release = notifier.onSystemNotificationOpened(opened);
  notifier.dispose();
  notifier.dispose();
  release();
  deliver?.("session-retired");
  expect(opened).not.toHaveBeenCalled();
  expect(stop).toHaveBeenCalledOnce();
  await expect(
    notifier.sendSystemNotification({
      id: "notification-retired",
      title: "Finished",
      body: "Done",
      target: "session-retired",
    }),
  ).resolves.toBe(false);
  expect(send).not.toHaveBeenCalled();
});

it("fences an individual subscription before its owner is disposed", () => {
  const stop = vi.fn();
  let deliver: ((target: string) => void) | undefined;
  const notifier = createSystemNotifier(
    centre({
      onOpened(open) {
        deliver = open;
        return stop;
      },
    }),
  );
  const opened = vi.fn();
  const release = notifier.onSystemNotificationOpened(opened);
  deliver?.("session-active");
  release();
  release();
  deliver?.("session-retired");
  notifier.dispose();
  expect(opened).toHaveBeenCalledExactlyOnceWith("session-active");
  expect(stop).toHaveBeenCalledOnce();
});

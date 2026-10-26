import { create } from "zustand";
import { type NotificationAuthorization } from "@/foundation/notificationAuthorization";
import {
  type NotificationCentre,
  type SystemNotification,
} from "@/features/notifications/application/notificationCentre";
export function createSystemNotifier(centre: NotificationCentre) {
  const useNotificationAuthorization = create<{ authorization: NotificationAuthorization }>()(
    () => ({ authorization: "default" }),
  );
  const subscriptions = new Set<() => void>();
  let disposed = false;
  let request = 0;
  async function update(
    read: () => Promise<NotificationAuthorization>,
  ): Promise<NotificationAuthorization> {
    const lease = ++request;
    const authorization = await read();
    if (!disposed && lease === request) useNotificationAuthorization.setState({ authorization });
    return authorization;
  }
  return {
    useNotificationAuthorization,
    refreshNotificationAuthorization: () => update(centre.authorization),
    requestNotificationAuthorization: () => update(centre.requestAuthorization),
    sendSystemNotification: (notification: SystemNotification) =>
      disposed ? Promise.resolve(false) : centre.send(notification),
    onSystemNotificationOpened(open: (target: string) => void) {
      if (disposed) return () => undefined;
      let active = true;
      const stop = centre.onOpened((target) => {
        if (active && !disposed) open(target);
      });
      const release = () => {
        if (!active) return;
        active = false;
        subscriptions.delete(release);
        stop();
      };
      subscriptions.add(release);
      return release;
    },
    dispose() {
      if (disposed) return;
      disposed = true;
      request++;
      for (const stop of subscriptions) stop();
      subscriptions.clear();
    },
  };
}
export type SystemNotifier = ReturnType<typeof createSystemNotifier>;

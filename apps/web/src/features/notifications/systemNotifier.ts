import { createStore } from "zustand/vanilla";
import { type NotificationAuthorization } from "@flame/component-sdk/notifications";
import {
  type NotificationCentre,
  type SystemNotification,
} from "@/features/notifications/application/notificationCentre";

type AuthorizationObservation = {
  kind: "read" | "request";
  result: Promise<NotificationAuthorization>;
};

export function createSystemNotifier(centre: NotificationCentre) {
  const authorization = createStore<NotificationAuthorization>(() => "default");
  const subscriptions = new Set<() => void>();
  let disposed = false;
  let pending: AuthorizationObservation | undefined;
  async function observeAuthorization(
    kind: AuthorizationObservation["kind"],
  ): Promise<NotificationAuthorization> {
    if (disposed) throw new Error("Notifications are disposed");
    if (pending && (pending.kind === "request" || pending.kind === kind))
      return pending.result;
    const result =
      kind === "request" ? centre.requestAuthorization() : centre.authorization();
    const observation: AuthorizationObservation = {
      kind,
      result: result
        .then((status) => {
          if (pending === observation) authorization.setState(status);
          return status;
        })
        .finally(() => {
          if (pending === observation) pending = undefined;
        }),
    };
    pending = observation;
    return observation.result;
  }
  return {
    authorization: authorization.getState,
    subscribeAuthorization: authorization.subscribe,
    refreshNotificationAuthorization: () => observeAuthorization("read"),
    requestNotificationAuthorization: () => observeAuthorization("request"),
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
      pending = undefined;
      for (const stop of subscriptions) stop();
      subscriptions.clear();
    },
  };
}
export type SystemNotifier = ReturnType<typeof createSystemNotifier>;

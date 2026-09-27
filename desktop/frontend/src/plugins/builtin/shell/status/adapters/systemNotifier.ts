import type { ClientHost } from "@/platform/host";
import {
  configureNotificationCentre,
  type NotificationCentre,
} from "../application/ports/notificationCentre";

export function installNotificationCentre(
  host: Pick<
    ClientHost,
    | "notificationAuthorization"
    | "requestNotificationAuthorization"
    | "sendNotification"
    | "onNotificationOpened"
  >,
): () => void {
  const centre: NotificationCentre = {
    authorization: () => host.notificationAuthorization(),
    requestAuthorization: () => host.requestNotificationAuthorization(),
    send: (notification) => host.sendNotification(notification),
    onOpened(open) {
      let stopHost: (() => void) | undefined;
      let stopped = false;
      void host.onNotificationOpened(open).then((stop) => {
        if (stopped) stop();
        else stopHost = stop;
      });
      return () => {
        stopped = true;
        stopHost?.();
      };
    },
  };
  return configureNotificationCentre(centre);
}

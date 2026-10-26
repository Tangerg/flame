import { type ClientHost } from "@/platform/host";
import { type NotificationCentre } from "@/features/notifications/application/notificationCentre";
export function createNotificationCentre(
  host: Pick<
    ClientHost,
    | "notificationAuthorization"
    | "requestNotificationAuthorization"
    | "sendNotification"
    | "onNotificationOpened"
  >,
): NotificationCentre {
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
  return centre;
}

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
  endpoint: () => string,
  registrationFailed: (error: unknown) => void,
): NotificationCentre {
  const centre: NotificationCentre = {
    authorization: () => host.notificationAuthorization(),
    requestAuthorization: () => host.requestNotificationAuthorization(),
    send: (notification) =>
      host.sendNotification({
        ...notification,
        target: JSON.stringify([endpoint(), notification.target]),
      }),
    onOpened(open) {
      let stopHost: (() => void) | undefined;
      let stopped = false;
      void host
        .onNotificationOpened((target) => {
          if (stopped) return;
          let address: unknown;
          try {
            address = JSON.parse(target);
          } catch {
            return;
          }
          if (
            Array.isArray(address) &&
            address.length === 2 &&
            address[0] === endpoint() &&
            typeof address[1] === "string"
          )
            open(address[1]);
        })
        .then(
          (stop) => {
            if (stopped) stop();
            else stopHost = stop;
          },
          (error: unknown) => {
            if (!stopped) registrationFailed(error);
          },
        );
      return () => {
        stopped = true;
        stopHost?.();
      };
    },
  };
  return centre;
}

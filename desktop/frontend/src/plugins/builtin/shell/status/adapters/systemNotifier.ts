import { getContainer } from "@/main/container";
import {
  configureNotificationCentre,
  type NotificationCentre,
} from "../application/ports/notificationCentre";

function createNotificationCentre(): NotificationCentre {
  const host = () => getContainer().host;
  return {
    authorization: () => host().notificationAuthorization(),
    requestAuthorization: () => host().requestNotificationAuthorization(),
    send: (notification) => host().sendNotification(notification),
    onOpened(open) {
      let stopHost: (() => void) | undefined;
      let stopped = false;
      void host()
        .onNotificationOpened(open)
        .then((stop) => {
          if (stopped) stop();
          else stopHost = stop;
        });
      return () => {
        stopped = true;
        stopHost?.();
      };
    },
  };
}

export function installNotificationCentre(): () => void {
  return configureNotificationCentre(createNotificationCentre());
}

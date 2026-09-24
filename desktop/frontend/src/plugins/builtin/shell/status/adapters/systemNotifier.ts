import { getContainer } from "@/main/container";
import {
  configureNotificationCentre,
  type NotificationAuthorization,
  type NotificationCentre,
} from "../application/ports/notificationCentre";

// The packaged app asks the desktop host, whose platform notification centre works
// where the webview has no Web Notifications API; a browser has no host and answers
// through the Web API instead. Each environment has exactly one owner.

function webAuthorization(): NotificationAuthorization {
  return typeof Notification === "undefined" ? "unsupported" : Notification.permission;
}

function createNotificationCentre(): NotificationCentre {
  let openTarget: ((target: string) => void) | undefined;
  const desktop = () => getContainer().desktop;
  return {
    async authorization() {
      return (await desktop().notificationAuthorization()) ?? webAuthorization();
    },
    async requestAuthorization() {
      const host = await desktop().requestNotificationAuthorization();
      if (host !== null) return host;
      if (typeof Notification === "undefined") return "unsupported";
      return Notification.permission === "default"
        ? await Notification.requestPermission()
        : Notification.permission;
    },
    async send(notification) {
      const sent = await desktop().sendNotification(notification);
      if (sent !== null) return sent;
      if (webAuthorization() !== "granted") return false;
      const shown = new Notification(notification.title, {
        body: notification.body,
        tag: notification.id,
      });
      shown.onclick = () => {
        shown.close();
        openTarget?.(notification.target);
      };
      return true;
    },
    onOpened(open) {
      openTarget = open;
      let stopHost: (() => void) | undefined;
      let stopped = false;
      void desktop()
        .onNotificationOpened(open)
        .then((stop) => {
          if (stopped) stop();
          else stopHost = stop;
        });
      return () => {
        stopped = true;
        stopHost?.();
        if (openTarget === open) openTarget = undefined;
      };
    },
  };
}

export function installNotificationCentre(): () => void {
  return configureNotificationCentre(createNotificationCentre());
}

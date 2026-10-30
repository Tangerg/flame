import { type NotificationAuthorization } from "@/foundation/notificationAuthorization";
import { downloadBlob } from "@/platform/download";
import { type ClientHost } from "@/platform/host";

function browserAuthorization(): NotificationAuthorization {
  return typeof Notification === "undefined" ? "unsupported" : Notification.permission;
}

export function createBrowserHost(): ClientHost {
  const listeners = new Set<(target: string) => void>();
  return {
    kind: "web",
    async bootstrap() {
      const endpoint = window.location.origin;
      if (!/^https?:\/\//.test(endpoint)) {
        throw new Error("the web client must be served over HTTP or HTTPS");
      }
      return { runtime: { endpoint }, localFilesystemEndpoint: null };
    },
    chooseWorkingDirectory: async () => null,
    async saveFile({ filename, bytes, mime }) {
      if (!filename || filename === "." || filename === ".." || /[/\\:\p{Cc}]/u.test(filename)) {
        throw new Error("The file name must be a basename without control characters.");
      }
      downloadBlob(filename, new Blob([new Uint8Array(bytes)], { type: mime }));
      return "requested";
    },
    windowChrome: async () => null,
    async revealWindow() {
      window.focus();
    },
    revealPath: async () => false,
    openPath: async () => false,
    notificationAuthorization: async () => browserAuthorization(),
    async requestNotificationAuthorization() {
      if (typeof Notification === "undefined") return "unsupported";
      return Notification.permission === "default"
        ? Notification.requestPermission()
        : Notification.permission;
    },
    async sendNotification(notification) {
      if (browserAuthorization() !== "granted") return false;
      const shown = new Notification(notification.title, {
        body: notification.body,
        tag: notification.id,
      });
      shown.onclick = () => {
        shown.close();
        if (notification.target) for (const listener of listeners) listener(notification.target);
      };
      return true;
    },
    async onNotificationOpened(listener) {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
  };
}

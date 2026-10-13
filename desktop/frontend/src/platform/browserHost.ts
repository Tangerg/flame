import { z } from "zod";
import type { NotificationAuthorization } from "@/foundation/notificationAuthorization";
import { downloadBlob } from "./download";
import type { ClientHost } from "./host";

const InlineImageSchema = z
  .string()
  .regex(/^data:image\/(png|jpeg|gif|webp);base64,[A-Za-z0-9+/]+=*$/);

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
    async saveImage(source) {
      const image = InlineImageSchema.parse(source);
      const mime = image.slice(5, image.indexOf(";"));
      const bytes = Uint8Array.from(atob(image.slice(image.indexOf(",") + 1)), (char) =>
        char.charCodeAt(0),
      );
      downloadBlob(
        `flame-image.${mime === "image/jpeg" ? "jpg" : mime.slice(6)}`,
        new Blob([bytes], { type: mime }),
      );
      return true;
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

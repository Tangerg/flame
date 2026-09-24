import { create } from "zustand";

export type NotificationPermissionState = "unsupported" | "default" | "granted" | "denied";

function readPermission(): NotificationPermissionState {
  return typeof Notification === "undefined" ? "unsupported" : Notification.permission;
}

export const useNotificationPermission = create<{ permission: NotificationPermissionState }>()(
  () => ({ permission: readPermission() }),
);

export async function requestNotificationPermission(): Promise<NotificationPermissionState> {
  if (typeof Notification === "undefined") return "unsupported";
  const permission =
    Notification.permission === "default"
      ? await Notification.requestPermission()
      : Notification.permission;
  useNotificationPermission.setState({ permission });
  return permission;
}

interface OsNotifyOptions {
  body?: string;
  tag?: string;
  onClick?: () => void;
}

export function osNotify(title: string, opts?: OsNotifyOptions): boolean {
  if (readPermission() !== "granted") return false;
  try {
    const notification = new Notification(title, { body: opts?.body, tag: opts?.tag });
    notification.onclick = () => {
      notification.close();
      opts?.onClick?.();
    };
    return true;
  } catch (error) {
    console.error("[notify] system notification failed:", error);
    return false;
  }
}

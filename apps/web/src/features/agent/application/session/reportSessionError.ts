import { t } from "@/lib/i18n/index";
import { type NotificationFeed } from "@/features/notifications/public/notifications";
export type SessionMutation = "create" | "delete" | "fork" | "favorite" | "relocate";

export function reportSessionError(
  notifications: NotificationFeed,
  action: SessionMutation,
  err: unknown,
  description?: string,
): void {
  console.error(`[session] ${action} failed:`, err);
  notifications.notify("error", t(`session.error.${action}`), { description, source: "session" });
}

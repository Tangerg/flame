import { t } from "@/lib/i18n";
import { notifyError } from "@/plugins/sdk";

export type SessionMutation = "create" | "delete" | "rename" | "fork" | "favorite" | "relocate";

export function reportSessionError(
  action: SessionMutation,
  err: unknown,
  description?: string,
): void {
  console.error(`[session] ${action} failed:`, err);
  notifyError(t(`session.error.${action}`), { description, source: "session" });
}

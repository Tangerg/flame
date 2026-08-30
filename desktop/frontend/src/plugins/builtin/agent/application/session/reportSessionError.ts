// Every session mutation NOTIFIES on failure: a silent console.error reads as "the click did
// nothing". The console still keeps the raw error object.

import { t } from "@/lib/i18n";
import { notifyError } from "@/plugins/sdk";

/** The mutations that report through here. A closed set because each one names
 *  a localized catalog key rather than interpolating an English action. */
export type SessionMutation = "create" | "delete" | "rename" | "fork" | "favorite" | "relocate";

export function reportSessionError(
  action: SessionMutation,
  err: unknown,
  description?: string,
): void {
  console.error(`[session] ${action} failed:`, err);
  notifyError(t(`session.error.${action}`), { description, source: "session" });
}

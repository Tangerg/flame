// Separate from the toaster: a vanished toast cannot answer "did anything fail?".

import type { NotificationEntry, NotificationLevel } from "./types";
import { toast } from "sonner";
import { create } from "zustand";
import { ExactSequence } from "@/foundation/exactSequence";

const MAX_ENTRIES = 200;

interface NotificationStoreState {
  log: NotificationEntry[];
}

interface NotificationStoreActions {
  push: (entry: { plugin: string; level: NotificationLevel; message: string }) => NotificationEntry;
  dismiss: (id: string) => void;
  clearAll: () => void;
}

const notificationIds = new ExactSequence();

export const useNotificationStore = create<NotificationStoreState & NotificationStoreActions>(
  (set, get) => ({
    log: [],

    push({ plugin, level, message }) {
      const id = notificationIds.issue().toString();
      const entry: NotificationEntry = {
        id,
        plugin,
        level,
        message,
        timestamp: Date.now(),
      };
      const next = [...get().log, entry];
      const trimmed = next.length > MAX_ENTRIES ? next.slice(next.length - MAX_ENTRIES) : next;
      set({ log: trimmed });
      return entry;
    },

    dismiss(id) {
      set({
        log: get().log.map((e) => (e.id === id ? { ...e, dismissed: true } : e)),
      });
    },

    clearAll() {
      set({ log: [] });
    },
  }),
);

// The non-plugin twin of `host.notify`: a durable feed entry PLUS a toast. Success stays
// toast-only — feedback on an action just watched, not an event worth re-reading.

/** An IDENTIFIER, not copy, so untranslated. CLOSED because as a `string` one typo opens a
 *  second silent attribution bucket that reads as a new subsystem. */
export type NotifySource =
  | "composer"
  | "events"
  | "goal"
  | "import"
  | "mcp"
  | "knowledge"
  | "project"
  | "render"
  | "session"
  | "setup"
  | "skills";

export interface NotifyOptions {
  /** Secondary line on the toast; folded into the feed entry's message. */
  description?: string;
  /** Feed attribution (the Notifications view's "{source} · time" line).
   *  Defaults to "app". */
  source?: NotifySource;
}

/** The transient half of every notification, whoever raised it. There is no
 *  `notifyWarn` below: "warn" reaches a toast only from a plugin's `host.notify`. */
const TOAST_BY_LEVEL: Record<NotificationLevel, typeof toast.info> = {
  info: toast.info,
  warn: toast.warning,
  error: toast.error,
};

function notify(level: "info" | "error", message: string, opts?: NotifyOptions): void {
  useNotificationStore.getState().push({
    plugin: opts?.source ?? "app",
    level,
    message: opts?.description ? `${message} — ${opts.description}` : message,
  });
  TOAST_BY_LEVEL[level](message, opts?.description ? { description: opts.description } : undefined);
}

export function notifyInfo(message: string, opts?: NotifyOptions): void {
  notify("info", message, opts);
}
export function notifyError(message: string, opts?: NotifyOptions): void {
  notify("error", message, opts);
}

/** The feed entry is written BEFORE the toast so anything reacting to the toast can
 *  cross-reference it. */
export function notifyFrom(plugin: string, message: string, level: NotificationLevel): void {
  useNotificationStore.getState().push({ plugin, level, message });
  TOAST_BY_LEVEL[level](message);
}

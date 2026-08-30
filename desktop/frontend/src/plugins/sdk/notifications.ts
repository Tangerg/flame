// Separate from the transient toaster because a toast that has vanished cannot answer "did
// anything fail?", and because views can read this without subscribing to DOM events.

import type { NotificationEntry, NotificationLevel } from "./types";
import { dispatchToast } from "./hostToast";
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

// The non-plugin twin of `host.notify`, with the same contract: a durable feed entry PLUS a
// transient toast. Success confirmations stay toast-only — they are feedback on an action
// the user just watched succeed, not events worth re-reading.

/**
 * An IDENTIFIER, not copy: it renders in the same column as a plugin's name, so it stays
 * untranslated for the same reason plugin ids do. CLOSED because as a `string` one typo
 * opens a second, silent attribution bucket that reads as a new subsystem.
 */
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

// The app's own notify helpers speak two of the feed's three levels — there is no
// notifyWarn, so "warn" reaches the feed only from a plugin's host.notify. Stated
// as a narrowing of the feed's vocabulary rather than a second list of words.
type Level = Extract<NotificationLevel, "info" | "error">;

const TOAST_BY_LEVEL: Record<Level, typeof toast.info> = {
  info: toast.info,
  error: toast.error,
};

function notify(level: Level, message: string, opts?: NotifyOptions): void {
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

/**
 * A plugin's own notification, attributed to it in the feed and reaching "warn", which the
 * app-side helpers deliberately do not. The feed entry is written BEFORE the toast so
 * anything reacting to the toast can cross-reference it, and the toast goes out as an event
 * so this path pulls no React portal machinery into the SDK.
 */
export function notifyFrom(plugin: string, message: string, level: NotificationLevel): void {
  useNotificationStore.getState().push({ plugin, level, message });
  dispatchToast(message, level);
}

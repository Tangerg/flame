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

export type NotifySource =
  | "agentMemory"
  | "composer"
  | "events"
  | "goal"
  | "import"
  | "mcp"
  | "project"
  | "render"
  | "session"
  | "setup"
  | "skills";

export interface NotifyOptions {
  description?: string;
  source?: NotifySource;
}

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

export function notifyFrom(plugin: string, message: string, level: NotificationLevel): void {
  useNotificationStore.getState().push({ plugin, level, message });
  TOAST_BY_LEVEL[level](message);
}

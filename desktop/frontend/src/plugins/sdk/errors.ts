import { create } from "zustand";
import { ExactSequence } from "@/foundation/exactSequence";

export type PluginErrorSource = "setup" | "render" | "events" | "command" | "other";

export interface PluginError {
  id: string;
  timestamp: number;
  plugin: string;
  source: PluginErrorSource;
  message: string;
  detail?: string;
}

interface ErrorStoreState {
  log: PluginError[];
}

interface ErrorStoreActions {
  push: (e: Omit<PluginError, "id" | "timestamp">) => void;
  clearFor: (plugin: string) => void;
  clearAll: () => void;
}

const MAX_ENTRIES = 200;
const pluginErrorIds = new ExactSequence();

export const usePluginErrorStore = create<ErrorStoreState & ErrorStoreActions>((set, get) => ({
  log: [],

  push({ plugin, source, message, detail }) {
    const id = pluginErrorIds.issue().toString();
    const next = [...get().log, { id, timestamp: Date.now(), plugin, source, message, detail }];
    const trimmed = next.length > MAX_ENTRIES ? next.slice(next.length - MAX_ENTRIES) : next;
    set({ log: trimmed });
  },

  clearFor(plugin) {
    set({ log: get().log.filter((e) => e.plugin !== plugin) });
  },

  clearAll() {
    set({ log: [] });
  },
}));

export function reportPluginError(
  plugin: string,
  source: PluginErrorSource,
  err: unknown,
  detail?: string,
): void {
  const message = err instanceof Error ? err.message : String(err);
  const trace = detail ?? (err instanceof Error ? err.stack : undefined);
  usePluginErrorStore.getState().push({ plugin, source, message, detail: trace });
}

export function safeCall(fn: () => void, tag: string): void {
  try {
    fn();
  } catch (err) {
    console.error(tag, err);
  }
}

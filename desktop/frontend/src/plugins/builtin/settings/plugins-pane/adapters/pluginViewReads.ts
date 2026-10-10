import type { FlameClient } from "@flame/runtime-contract/client";
import type {
  AgentMemoryItem,
  Page,
  Schedule,
  ReadPluginViewRequest,
  TrajectoryEntry,
} from "@flame/runtime-contract/wire";
import type { PluginViewReads, MemoryViewTarget } from "../application/pluginView";

export function createTrajectoryViewReads(
  plugins: Pick<FlameClient["plugins"], "readView" | "readTrajectory">,
  binding: Readonly<ReadPluginViewRequest>,
  sessionId: string,
  includeDescendants: boolean,
): PluginViewReads<TrajectoryEntry> {
  const view = { ...binding };
  return createViewReads(plugins, view, (cursor, signal) =>
    plugins.readTrajectory(
      {
        ...view,
        sessionId,
        ...(includeDescendants ? { includeDescendants: true } : {}),
        cursor,
        limit: 100,
      },
      signal,
    ),
  );
}

export function createMemoryViewReads(
  plugins: Pick<FlameClient["plugins"], "readView" | "readMemory">,
  binding: Readonly<ReadPluginViewRequest>,
  target: Readonly<MemoryViewTarget>,
): PluginViewReads<AgentMemoryItem> {
  const view = { ...binding };
  const captured = {
    ...target,
    ...(target.workspace ? { workspace: { ...target.workspace } } : {}),
  };
  return createViewReads(plugins, view, (cursor, signal) =>
    plugins.readMemory({ ...view, ...captured, cursor }, signal),
  );
}

export function createScheduleViewReads(
  plugins: Pick<FlameClient["plugins"], "readView" | "readSchedules">,
  binding: Readonly<ReadPluginViewRequest>,
): PluginViewReads<Schedule> {
  const view = { ...binding };
  return createViewReads(plugins, view, (cursor, signal) =>
    plugins.readSchedules({ ...view, cursor }, signal),
  );
}

function createViewReads<T>(
  plugins: Pick<FlameClient["plugins"], "readView">,
  view: ReadPluginViewRequest,
  read: (cursor: string | undefined, signal: AbortSignal) => Promise<Page<T>>,
): PluginViewReads<T> {
  return {
    read,
    async load(signal) {
      const loading = new AbortController();
      const owned = AbortSignal.any([signal, loading.signal]);
      const pending = [plugins.readView(view, owned), read(undefined, owned)] as const;
      try {
        const [resource, initial] = await Promise.all(pending);
        return { html: resource.html, initial };
      } catch (error) {
        loading.abort(error);
        await Promise.allSettled(pending);
        throw error;
      }
    },
  };
}

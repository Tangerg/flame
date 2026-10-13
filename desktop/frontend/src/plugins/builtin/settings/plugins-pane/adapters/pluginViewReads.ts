import type { FlameClient } from "@flame/runtime-contract/client";
import type { Page, Schedule, ReadPluginViewRequest } from "@flame/runtime-contract/wire";
import type { PluginViewReads } from "../application/pluginView";

export function createScheduleViewReads(
  plugins: Pick<FlameClient["plugins"], "readView" | "readSchedules">,
  binding: Readonly<ReadPluginViewRequest>,
): PluginViewReads<Page<Schedule>> {
  const view = Object.freeze({ ...binding });
  return createViewReads(plugins, view, (cursor, signal) =>
    plugins.readSchedules({ ...view, cursor }, signal),
  );
}

function createViewReads<T>(
  plugins: Pick<FlameClient["plugins"], "readView">,
  view: ReadPluginViewRequest,
  read: (cursor: string | undefined, signal: AbortSignal) => Promise<T>,
): PluginViewReads<T> {
  return {
    binding: view,
    resource: async (signal) => (await plugins.readView(view, signal)).html,
    initial: (signal) => read(undefined, signal),
    read,
  };
}

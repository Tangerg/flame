import type { FlameClient } from "@flame/runtime-contract/client";
import type { ReadPluginRendererRequest } from "@flame/runtime-contract/wire";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import type { PluginViewReads } from "../application/pluginView";

export interface ToolResultSnapshot {
  format: "flame.tool-result.v1";
  tool: ToolCall;
}

export function createRecordedRendererReads<T extends ToolResultSnapshot>(
  plugins: Pick<FlameClient["plugins"], "readRenderer">,
  binding: Readonly<ReadPluginRendererRequest>,
  record: T,
): PluginViewReads<T> {
  const request = Object.freeze({ ...binding });
  const snapshot = structuredClone(record);
  let admittedHTML: string | undefined;
  return {
    binding: request,
    async resource(signal) {
      const resource = await plugins.readRenderer(request, signal);
      admittedHTML = resource.html;
      return resource.html;
    },
    async initial() {
      return structuredClone(snapshot);
    },
    async read(cursor, signal) {
      if (cursor !== undefined) throw new Error("Captured renderer records have no continuation");
      const resource = await plugins.readRenderer(request, signal);
      if (resource.html !== admittedHTML)
        throw new Error("The captured record renderer resource changed");
      return structuredClone(snapshot);
    },
  };
}

import type { FlameClient } from "@flame/runtime-contract/client";
import type { ReadPluginRendererRequest } from "@flame/runtime-contract/wire";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import type { PluginViewReads } from "../application/pluginView";

export interface ToolResultSnapshot {
  format: "flame.tool-result.v1";
  tool: ToolCall;
}

export function createToolRendererReads(
  plugins: Pick<FlameClient["plugins"], "readRenderer">,
  binding: Readonly<ReadPluginRendererRequest>,
  tool: Readonly<ToolCall>,
): PluginViewReads<ToolResultSnapshot> {
  const request = { ...binding };
  const snapshot: ToolResultSnapshot = {
    format: "flame.tool-result.v1",
    tool: structuredClone(tool),
  };
  let admittedHTML: string | undefined;
  return {
    async load(signal) {
      const resource = await plugins.readRenderer(request, signal);
      admittedHTML = resource.html;
      return { html: resource.html, initial: structuredClone(snapshot) };
    },
    async read(cursor, signal) {
      if (cursor !== undefined) throw new Error("Recorded tool results have no continuation");
      const resource = await plugins.readRenderer(request, signal);
      if (resource.html !== admittedHTML)
        throw new Error("The recorded result renderer resource changed");
      return structuredClone(snapshot);
    },
  };
}

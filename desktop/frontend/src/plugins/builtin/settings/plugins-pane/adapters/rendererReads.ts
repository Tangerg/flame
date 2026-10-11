import type { FlameClient } from "@flame/runtime-contract/client";
import type { ReadPluginRendererRequest } from "@flame/runtime-contract/wire";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import type { WorkspaceFileRendererProps } from "@/plugins/sdk/types/workspace";
import type { PluginViewReads } from "../application/pluginView";

export interface ToolResultSnapshot {
  format: "flame.tool-result.v1";
  tool: ToolCall;
}

export interface FileSnapshot {
  format: "flame.file.v1";
  file: Omit<WorkspaceFileRendererProps, "fallback">;
}

export function createRecordedRendererReads<T extends ToolResultSnapshot | FileSnapshot>(
  plugins: Pick<FlameClient["plugins"], "readRenderer">,
  binding: Readonly<ReadPluginRendererRequest>,
  record: T,
): PluginViewReads<T> {
  const request = { ...binding };
  const snapshot = structuredClone(record);
  let admittedHTML: string | undefined;
  return {
    async load(signal) {
      const resource = await plugins.readRenderer(request, signal);
      admittedHTML = resource.html;
      return { html: resource.html, initial: structuredClone(snapshot) };
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

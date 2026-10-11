import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import { TOOL_RESULT_RENDERER, useExtensionPoint, type ToolPreviewProps } from "@/plugins/sdk";
import { ToolInspector } from "./ToolInspector";
import { createElement } from "react";
import { selectToolResultRenderer } from "../application/toolResultRenderer";

export function ToolPreview({ tool }: ToolPreviewProps) {
  const renderers = useExtensionPoint(TOOL_RESULT_RENDERER);
  const renderer = selectToolResultRenderer(renderers, tool);
  if (!renderer) {
    return <ToolInspector tool={tool} />;
  }
  return (
    <PluginBoundary
      plugin={renderer.id}
      label={renderer.title}
      fallback={<ToolInspector tool={tool} />}
    >
      {createElement(renderer.component, { tool, fallback: <ToolInspector tool={tool} /> })}
    </PluginBoundary>
  );
}

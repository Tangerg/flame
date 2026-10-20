import { PluginBoundary } from "@/extensions/host/PluginBoundary";
import { TOOL_RESULT_RENDERER, type ToolPreviewProps } from "@/features/chat/public/tools/types";
import { useExtensionPoint } from "@/extensions/selectors/extensions";
import { ToolInspector } from "@/features/chat/tools/ui/ToolInspector";
import { createElement } from "react";
import { selectToolResultRenderer } from "@/features/chat/tools/application/toolResultRenderer";
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

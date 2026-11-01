import { PluginBoundary } from "@/extensions/host/PluginBoundary";
import { TOOL_RESULT_RENDERER, type ToolPreviewProps } from "@/features/chat/public/tools/types";
import { useExtensionPoint } from "@/extensions/selectors/extensions";
import { ToolInspector } from "@/features/chat/tools/ui/ToolInspector";
import { createElement } from "react";
import { useRendererChoice } from "@/features/plugins/public/renderers";
import {
  toolRendererCandidates,
  toolRendererTarget,
} from "@/features/chat/tools/application/toolResultRenderer";
export function ToolPreview({ tool }: ToolPreviewProps) {
  const renderers = useExtensionPoint(TOOL_RESULT_RENDERER);
  const { renderer, control } = useRendererChoice(
    "tool",
    toolRendererTarget(tool),
    toolRendererCandidates(renderers, tool),
  );
  return (
    <>
      {control}
      <ToolInspector tool={tool}>
        {renderer
          ? (fallback) => (
              <PluginBoundary plugin={renderer.id} label={renderer.title} fallback={fallback}>
                {createElement(renderer.component, { tool, fallback })}
              </PluginBoundary>
            )
          : undefined}
      </ToolInspector>
    </>
  );
}

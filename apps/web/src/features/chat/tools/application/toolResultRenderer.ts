import { type ToolResultRendererSpec } from "@/features/chat/public/tools/types";
import { type ToolCall } from "@/features/agent/public/sessionView";
export function toolRendererTarget(tool: ToolCall): string {
  return tool.source?.modelName ?? tool.name;
}
export function toolRendererCandidates(
  renderers: readonly ToolResultRendererSpec[],
  tool: ToolCall,
): ToolResultRendererSpec[] {
  if (tool.status === "running" || tool.result === undefined) return [];
  const selectorName = toolRendererTarget(tool);
  const named = renderers.filter(
    ({ selector }) => selector.type === "toolName" && selector.name === selectorName,
  );
  if (named.length) return named;
  const shape = tool.resultShape;
  if (!shape) return [];
  return renderers.filter(
    ({ selector }) => selector.type === "resultShape" && selector.shape === shape,
  );
}

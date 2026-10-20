import { type ToolResultRendererSpec } from "@/features/chat/public/tools/types";
import { type ToolCall } from "@/features/agent/public/sessionView";
import { toolResultShape } from "@/features/chat/public/tools/toolResult";
export function selectToolResultRenderer(
  renderers: readonly ToolResultRendererSpec[],
  tool: ToolCall,
): ToolResultRendererSpec | undefined {
  if (tool.status === "running" || tool.result === undefined) return undefined;
  const named = renderers.filter(
    ({ selector }) => selector.type === "toolName" && selector.name === tool.name,
  );
  if (named.length) return named.length === 1 ? named[0] : undefined;
  const shape = toolResultShape(tool.result);
  if (!shape) return undefined;
  const shaped = renderers.filter(
    ({ selector }) => selector.type === "resultShape" && selector.name === shape,
  );
  return shaped.length === 1 ? shaped[0] : undefined;
}

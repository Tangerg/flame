import type { ToolResultRendererSpec } from "@/plugins/sdk/types/tool";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { toolResultShape } from "@/plugins/sdk/toolResult";

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

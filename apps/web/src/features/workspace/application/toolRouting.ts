import { type WorkspaceAccess } from "@/features/workspace/public/workspace";
import { type ToolCall } from "@/features/agent/public/sessionView";
import { openWorkspaceFile } from "@/features/workspace/application/navigation";

export function hasWorkspaceViewForTool(tool: ToolCall): boolean {
  return tool.category === "read" && tool.fnKind === "path";
}

export function openWorkspaceViewForTool(
  workspace: WorkspaceAccess,
  tool: ToolCall,
  origin?: HTMLElement,
): void {
  if (hasWorkspaceViewForTool(tool))
    openWorkspaceFile(workspace, tool.fn, tool.range?.start, origin);
}

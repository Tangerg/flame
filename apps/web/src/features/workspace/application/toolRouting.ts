import { type ToolCall } from "@/features/agent/public/sessionView";
import { toolCategory } from "@/features/agent/public/viewState";
import {
  openWorkspaceDiffForFile,
  openWorkspaceFile,
} from "@/features/workspace/application/navigation";
function toolDestination(tool: ToolCall): "diff" | "file" | null {
  const category = toolCategory(tool.name);
  if (category === "fileEdit") return "diff";
  if (category === "read" && tool.fnKind === "path") return "file";
  return null;
}

export function hasWorkspaceViewForTool(tool: ToolCall): boolean {
  return toolDestination(tool) !== null;
}

const DESTINATION_LABEL = {
  diff: "tool.open.worktreeDiff",
  file: "tool.open.currentFile",
} as const;

export function workspaceViewLabelForTool(tool: ToolCall): string {
  const destination = toolDestination(tool);
  return destination ? DESTINATION_LABEL[destination] : "workspace.view.openBeside";
}

export function openWorkspaceViewForTool(tool: ToolCall, origin?: HTMLElement): void {
  switch (toolDestination(tool)) {
    case "diff":
      openWorkspaceDiffForFile(tool.fnKind === "path" ? tool.fn : "", origin);
      break;
    case "file":
      openWorkspaceFile(tool.fn, tool.range?.start, origin);
      break;
  }
}

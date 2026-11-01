import { type WorkspaceAccess } from "@/features/workspace/public/workspace";
import { definePlugin } from "@/extensions/definePlugin";
import { TOOL_VIEW_OPENER } from "@/features/chat/public/tools/types";
import {
  hasWorkspaceViewForTool,
  openWorkspaceViewForTool,
} from "@/features/workspace/application/toolRouting";
export default function createToolViewOpener(workspaceAccess: WorkspaceAccess) {
  return definePlugin({
    name: "flame.builtin.workspace.tool-view-opener",
    setup(ctx) {
      ctx.contribute(TOOL_VIEW_OPENER, {
        id: "workspace-tool-view",
        order: 0,
        predicate: hasWorkspaceViewForTool,
        label: () => "tool.open.currentFile",
        open: (tool, origin) => openWorkspaceViewForTool(workspaceAccess, tool, origin),
      });
    },
  });
}

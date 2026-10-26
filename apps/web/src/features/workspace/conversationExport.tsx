import { type WorkspaceAccess } from "@/features/workspace/public/workspace";
import { COMMAND } from "@/features/command/public/extensionPoints";
import { definePlugin } from "@/extensions/definePlugin";

import { conversationExportCommands } from "@/features/workspace/application/conversationExportCommands";
export default function createConversationExportPlugin(workspaceAccess: WorkspaceAccess) {
  return definePlugin({
    name: "flame.builtin.conversation-export",
    setup(ctx) {
      for (const command of conversationExportCommands({
        exportMarkdown: workspaceAccess.archive.exportConversationMarkdown,
        exportJson: workspaceAccess.archive.exportConversationJson,
        exportTrajectory: workspaceAccess.archive.exportSessionTrajectory,
        importJson: workspaceAccess.archive.importConversationJson,
      })) {
        ctx.contribute(COMMAND, command);
      }
    },
  });
}

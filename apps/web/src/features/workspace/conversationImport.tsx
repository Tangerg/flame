import { type WorkspaceAccess } from "@/features/workspace/public/workspace";
import { COMMAND } from "@/features/command/public/extensionPoints";
import { definePlugin } from "@/extensions/definePlugin";

export default function createConversationImportPlugin(workspaceAccess: WorkspaceAccess) {
  return definePlugin({
    name: "flame.builtin.conversation-import",
    setup(ctx) {
      ctx.contribute(COMMAND, {
        id: "chat.import.json",
        label: "convExport.import",
        run: workspaceAccess.conversationImport.importConversationJson,
      });
    },
  });
}

import { COMMAND } from "@/features/command/public/extensionPoints";
import { definePlugin } from "@/extensions/definePlugin";
import {
  exportConversationJson,
  exportConversationMarkdown,
  exportSessionTrajectory,
  importConversationJson,
} from "@/features/workspace/public/conversationArchive";
import { conversationExportCommands } from "@/features/workspace/application/conversationExportCommands";
export default definePlugin({
  name: "flame.builtin.conversation-export",
  setup(ctx) {
    for (const command of conversationExportCommands({
      exportMarkdown: exportConversationMarkdown,
      exportJson: exportConversationJson,
      exportTrajectory: exportSessionTrajectory,
      importJson: importConversationJson,
    })) {
      ctx.contribute(COMMAND, command);
    }
  },
});

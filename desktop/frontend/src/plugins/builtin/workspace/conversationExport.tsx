import { COMMAND, definePlugin } from "@/plugins/sdk";
import {
  exportConversationJson,
  exportConversationMarkdown,
  exportSessionTrajectory,
  importConversationJson,
} from "@/plugins/builtin/workspace/public/conversationArchive";
import { conversationExportCommands } from "./application/conversationExportCommands";

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

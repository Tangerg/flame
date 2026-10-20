import { COMMAND } from "@/features/command/public/extensionPoints";
import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { openChatSearch } from "@/features/chat/search/application/openChatSearch";
import { ChatSearchOverlay } from "@/features/chat/search/ui/ChatSearchOverlay";
export default definePlugin({
  name: "flame.builtin.chat-search",
  setup(ctx) {
    contributeLayout(ctx, "app.overlay", {
      id: "chat-search",
      order: 50,
      component: ChatSearchOverlay,
    });
    ctx.contribute(COMMAND, {
      id: "chat.search",
      label: "command.chatSearch",
      combo: "Mod+F",
      run: openChatSearch,
    });
  },
});

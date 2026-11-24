import { COMMAND } from "@/features/command/public/extensionPoints";
import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { ChatSearchOpener } from "@/features/chat/search/application/openChatSearch";
import { ChatSearchOverlay } from "@/features/chat/search/ui/ChatSearchOverlay";
export default definePlugin({
  name: "flame.builtin.chat-search",
  setup(ctx) {
    const opener = new ChatSearchOpener();
    function InstalledChatSearch() {
      return <ChatSearchOverlay opener={opener} />;
    }
    contributeLayout(ctx.contributions, "chat.search", {
      id: "chat-search",
      order: 0,
      component: InstalledChatSearch,
    });
    ctx.contributions.add(COMMAND, {
      id: "chat.search",
      label: "command.chatSearch",
      combo: "Mod+F",
      run: () => opener.open(),
    });
  },
});

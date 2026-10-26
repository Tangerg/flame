import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { COMMAND } from "@/features/command/public/extensionPoints";
import { useSessionSearchStore } from "@/features/navigation/session-search/sessionSearchState";
import { SESSION_SEARCH_COMMAND } from "@/features/navigation/public/search/actions";
import { SessionSearch } from "@/features/navigation/session-search/SessionSearch";
export default definePlugin({
  name: "flame.builtin.session-search",
  setup(ctx) {
    contributeLayout(ctx, "app.overlay", {
      id: "session-search",
      order: 10,
      component: SessionSearch,
    });
    ctx.contribute(COMMAND, {
      id: SESSION_SEARCH_COMMAND,
      label: "command.searchChats",
      combo: "Mod+K",
      run: () => useSessionSearchStore.getState().toggle(),
    });
  },
});

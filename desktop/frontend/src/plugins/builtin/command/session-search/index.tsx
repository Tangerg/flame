import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { COMMAND } from "@/plugins/sdk/kernelPoints";
import { useSessionSearchStore } from "./application/sessionSearchState";
import { SESSION_SEARCH_COMMAND } from "./public/actions";
import { SessionSearch } from "./ui/SessionSearch";

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

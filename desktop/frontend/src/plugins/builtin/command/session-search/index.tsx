import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { SHORTCUT } from "@/plugins/sdk/kernelPoints";
import { sessionSearchLauncher } from "./application/ports/sessionSearchLauncher";
import { installSessionSearchLauncher } from "./adapters/sessionSearchLauncher";
import { SessionSearch } from "./ui/SessionSearch";

export default definePlugin({
  name: "flame.builtin.session-search",
  setup(ctx) {
    const disposeLauncher = installSessionSearchLauncher();
    contributeLayout(ctx, "app.overlay", {
      id: "session-search",
      order: 10,
      component: SessionSearch,
    });
    ctx.contribute(SHORTCUT, {
      key: "Mod+K",
      description: "shortcut.sessionSearch",
      allowInInputs: true,
      handler: (event) => {
        event.preventDefault();
        sessionSearchLauncher().toggle();
      },
    });
    ctx.cleanup(disposeLauncher);
  },
});

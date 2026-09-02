import { definePlugin } from "@/plugins/sdk";
import { AGENT_SESSIONS } from "@/plugins/builtin/agent/public/services";
import { installRecipeSlashCommands } from "./application/recipeSlashCommands";

export default definePlugin({
  name: "flame.builtin.recipes-slash",
  requires: { sessions: AGENT_SESSIONS },
  setup(ctx) {
    ctx.cleanup(installRecipeSlashCommands(ctx, ctx.sessions));
  },
});

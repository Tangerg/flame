import { AgentClientPage } from "@/pages/AgentClientPage";
import { definePlugin } from "@/extensions/definePlugin";
import { ROUTE } from "@/features/shell/public/routes";
export default definePlugin({
  name: "flame.builtin.main-route",
  setup(ctx) {
    ctx.contribute(ROUTE, { id: "main", path: "/", order: 0, component: AgentClientPage });
  },
});

import { AgentClientPage } from "@/pages/AgentClientPage";
import { definePlugin } from "@/plugins/sdk";
import { ROUTE } from "@/plugins/sdk/kernelPoints";

export default definePlugin({
  name: "flame.builtin.main-route",
  setup(ctx) {
    ctx.contribute(ROUTE, { id: "main", path: "/", order: 0, component: AgentClientPage });
  },
});

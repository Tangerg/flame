import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { ContextUsageGauge } from "./ui/ContextUsageGauge";

export default definePlugin({
  name: "flame.builtin.context-usage",
  setup(ctx) {
    contributeLayout(ctx, "composer.toolbar.start", {
      id: "context-usage",
      order: 3,
      component: ContextUsageGauge,
    });
  },
});

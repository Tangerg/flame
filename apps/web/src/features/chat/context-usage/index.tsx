import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { ContextUsageGauge } from "@/features/chat/context-usage/ui/ContextUsageGauge";
export default definePlugin({
  name: "flame.builtin.context-usage",
  setup(ctx) {
    contributeLayout(ctx, "composer.toolbar.start", {
      id: "context-usage",
      order: 4,
      component: ContextUsageGauge,
    });
  },
});

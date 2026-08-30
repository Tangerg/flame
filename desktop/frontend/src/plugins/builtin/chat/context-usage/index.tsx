// On the composer rather than the title bar: a fixed-size dial does not reflow the control
// row, and its numbers stay in the tooltip where they cost no layout at all.

import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { ContextUsageGauge } from "./ui/ContextUsageGauge";

export default definePlugin({
  name: "flame.builtin.context-usage",
  setup(ctx) {
    contributeLayout(ctx, "composer.toolbar.start", {
      // After the model it measures: the window reads as that control's consequence.
      id: "context-usage",
      order: 3,
      component: ContextUsageGauge,
    });
  },
});

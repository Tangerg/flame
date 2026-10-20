import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { SelectionQuote } from "@/features/composer/quote-selection/ui/SelectionQuote";
export default definePlugin({
  name: "flame.builtin.quote-selection",
  setup(ctx) {
    contributeLayout(ctx, "app.overlay", {
      id: "selection-quote",
      order: 60,
      component: SelectionQuote,
    });
  },
});

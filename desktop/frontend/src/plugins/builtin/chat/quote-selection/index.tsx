import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { SelectionQuote } from "./ui/SelectionQuote";

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

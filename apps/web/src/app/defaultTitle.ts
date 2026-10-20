import { definePlugin } from "@/extensions/definePlugin";
import { WINDOW } from "@/features/shell/public/window";
import { PRODUCT_NAME } from "@/app/product";
export const defaultTitle = definePlugin({
  name: "flame.builtin.default-title",
  requires: { window: WINDOW },
  setup(ctx) {
    ctx.window.setTitle(PRODUCT_NAME);
  },
});

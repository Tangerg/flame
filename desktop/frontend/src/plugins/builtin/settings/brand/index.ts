import { createElement, lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../kit";
import { BRAND_PANE } from "../kit/panes";

const BrandPane = lazy(() =>
  import("./ui/BrandPane").then(({ BrandPane }) => ({ default: BrandPane })),
);

export function createBrandPlugin(saveImage: (source: string) => Promise<boolean>) {
  return definePlugin({
    name: "flame.builtin.brand",
    setup(ctx) {
      registerSettingsPane(ctx, {
        id: BRAND_PANE,
        label: "settings.pane.brand",
        description: "settings.brand.description",
        keywords: [
          "settings.brand.mark.title",
          "settings.brand.companion.title",
          "settings.brand.plush.title",
          "settings.brand.color.title",
        ],
        group: "general",
        icon: "spark",
        order: 4,
        component: function BrandSettings() {
          return createElement(BrandPane, { saveImage });
        },
      });
    },
  });
}

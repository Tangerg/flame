import { createElement, lazy } from "react";
import { definePlugin } from "@/extensions/definePlugin";
import { registerSettingsPane } from "@/features/settings/public/registerSettingsPane";
import { BRAND_PANE } from "@/features/settings/public/paneIDs";
const BrandPane = lazy(() =>
  import("@/features/settings/brand/ui/BrandPane").then(({ BrandPane }) => ({
    default: BrandPane,
  })),
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

import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import { registerSettingsPane } from "../kit";
import { BRAND_ICONS_PANE } from "../kit/panes";

const IconGallery = lazy(() =>
  import("./ui/IconGallery").then(({ IconGallery }) => ({ default: IconGallery })),
);
const IconShowcase = lazy(() =>
  import("./ui/IconShowcase").then(({ IconShowcase }) => ({ default: IconShowcase })),
);

export default definePlugin({
  name: "flame.builtin.icon-gallery",
  setup(ctx) {
    ctx.contribute(WORKSPACE_VIEW, {
      id: "icon-gallery",
      title: "workspace.view.title.iconGallery",
      icon: "spark",
      order: 60,
      component: IconGallery,
    });

    registerSettingsPane(ctx, {
      id: BRAND_ICONS_PANE,
      label: "settings.pane.brandIcons",
      group: "advanced",
      icon: "image",
      order: 110,
      component: IconShowcase,
    });
  },
});

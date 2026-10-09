import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import { registerSettingsPane } from "../kit";
import { ICONS_PANE } from "../kit/panes";

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
      icon: "grid-2",
      order: 60,
      component: IconGallery,
    });

    registerSettingsPane(ctx, {
      id: ICONS_PANE,
      label: "settings.pane.icons",
      group: "advanced",
      icon: "grid-2",
      order: 110,
      component: IconShowcase,
    });
  },
});

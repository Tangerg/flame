// Built-in plugin: Appearance settings pane.
//
// Sections (theme / accent / contrast / font / shape / motion / language)
// live in sibling files. This file is only the plugin manifest.

import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../kit";
import { APPEARANCE_PANE } from "../kit/panes";
import { installBrowserFontAvailability } from "./adapters/browserFontAvailability";

const AppearancePane = lazy(() =>
  import("./ui/AppearancePane").then(({ AppearancePane }) => ({ default: AppearancePane })),
);

export default definePlugin({
  name: "flame.builtin.appearance",
  setup(ctx) {
    const disposeFonts = installBrowserFontAvailability();
    registerSettingsPane(ctx, {
      id: APPEARANCE_PANE,
      label: "settings.pane.appearance",
      description: "settings.appearance.hero",
      group: "general",
      icon: "sun",
      order: 0,
      component: AppearancePane,
    });
    ctx.cleanup(disposeFonts);
  },
});

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
      keywords: [
        "settings.theme",
        "settings.accent",
        "settings.contrast",
        "settings.customColors",
        "settings.density",
        "settings.font.ui",
        "settings.font.code",
        "settings.font.size",
        "settings.font.smoothing",
        "settings.language.label",
        "settings.motion",
        "settings.radius",
      ],
      description: "settings.appearance.hero",
      group: "general",
      icon: "sun",
      order: 0,
      component: AppearancePane,
    });
    ctx.cleanup(disposeFonts);
  },
});

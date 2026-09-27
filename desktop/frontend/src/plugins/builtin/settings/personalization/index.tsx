import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../kit";
import { PERSONALIZATION_PANE } from "../kit/panes";
import { installPersonalizationPreferencesPort } from "./adapters/uiPersonalizationPreferences";

const PersonalizationPane = lazy(() =>
  import("./ui/PersonalizationPane").then(({ PersonalizationPane }) => ({
    default: PersonalizationPane,
  })),
);

export default definePlugin({
  name: "flame.builtin.personalization",
  setup(ctx) {
    ctx.cleanup(installPersonalizationPreferencesPort());
    registerSettingsPane(ctx, {
      id: PERSONALIZATION_PANE,
      label: "settings.pane.personalization",
      keywords: ["settings.notifications", "settings.completionSound", "settings.streamReveal"],
      group: "general",
      icon: "user",
      order: 1,
      component: PersonalizationPane,
    });
  },
});

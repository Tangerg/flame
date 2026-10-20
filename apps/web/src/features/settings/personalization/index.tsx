import { lazy } from "react";
import { definePlugin } from "@/extensions/definePlugin";
import { registerSettingsPane } from "@/features/settings/public/registerSettingsPane";
import { PERSONALIZATION_PANE } from "@/features/settings/public/paneIDs";
const PersonalizationPane = lazy(() =>
  import("@/features/settings/ui/PersonalizationPane").then(({ PersonalizationPane }) => ({
    default: PersonalizationPane,
  })),
);

export default definePlugin({
  name: "flame.builtin.personalization",
  setup(ctx) {
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

import { lazy } from "react";
import { definePlugin } from "@/extensions/definePlugin";
import { registerSettingsPane } from "@/features/settings/public/registerSettingsPane";
import { APPROVALS_PANE } from "@/features/settings/public/paneIDs";
const ApprovalsPane = lazy(() =>
  import("@/features/approvals/ui/ApprovalsPane").then(({ ApprovalsPane }) => ({
    default: ApprovalsPane,
  })),
);

export default definePlugin({
  name: "flame.builtin.approvals-pane",
  setup(ctx) {
    registerSettingsPane(ctx, {
      id: APPROVALS_PANE,
      label: "settings.pane.approvals",
      keywords: ["approvals.mode", "approvals.rules"],
      group: "agent",
      icon: "shield",
      order: 55,
      component: ApprovalsPane,
    });
  },
});

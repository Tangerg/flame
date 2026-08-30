import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { APPROVALS_PANE } from "../public/panes";

const ApprovalsPane = lazy(() =>
  import("./ui/ApprovalsPane").then(({ ApprovalsPane }) => ({ default: ApprovalsPane })),
);

export default definePlugin({
  name: "flame.builtin.approvals-pane",
  setup(ctx) {
    registerSettingsPane(ctx, {
      id: APPROVALS_PANE,
      label: "settings.pane.approvals",
      group: "agent",
      icon: "shield",
      order: 55,
      component: ApprovalsPane,
    });
  },
});

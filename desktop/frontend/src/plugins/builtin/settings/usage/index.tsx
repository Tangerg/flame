import type { FlameClient } from "@flame/runtime-contract/client";
import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../kit";
import { USAGE_PANE } from "../kit/panes";
import { installUsageGateway } from "./adapters/runtimeUsageGateway";

const UsagePane = lazy(() =>
  import("./ui/UsagePane").then(({ UsagePane }) => ({ default: UsagePane })),
);

export function createUsagePlugin(runtimeClient: () => FlameClient) {
  return definePlugin({
    name: "flame.builtin.usage-pane",
    setup(ctx) {
      ctx.cleanup(installUsageGateway(runtimeClient));
      registerSettingsPane(ctx, {
        id: USAGE_PANE,
        label: "settings.pane.usage",
        group: "models",
        icon: "chart",
        order: 55,
        component: UsagePane,
      });
    },
  });
}

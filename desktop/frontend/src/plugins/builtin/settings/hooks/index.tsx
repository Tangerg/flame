import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../kit";
import { HOOKS_PANE } from "../kit/panes";
import { installHookTrustGateway } from "./adapters/runtimeHookTrustGateway";
import {
  RUNTIME_STREAM_PORTS,
  followRuntimeGeneration,
} from "@/plugins/builtin/runtime/public/ports";

const HooksPane = lazy(() =>
  import("./ui/HooksPane").then(({ HooksPane }) => ({ default: HooksPane })),
);

export default definePlugin({
  name: "flame.builtin.hooks-pane",
  requires: { runtime: RUNTIME_STREAM_PORTS },
  setup(ctx) {
    const gateway = installHookTrustGateway();
    const unsubscribeRuntime = followRuntimeGeneration(ctx.runtime, () =>
      gateway.replaceRuntimeGeneration(),
    );
    registerSettingsPane(ctx, {
      id: HOOKS_PANE,
      label: "settings.pane.hooks",
      group: "agent",
      icon: "lightning",
      order: 57,
      component: HooksPane,
    });
    ctx.cleanup(() => {
      unsubscribeRuntime();
      gateway.dispose();
    });
  },
});

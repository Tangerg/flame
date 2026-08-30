import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { HOOKS_PANE } from "../public/panes";
import { installHookTrustGateway } from "./adapters/runtimeHookTrustGateway";
import { RUNTIME_STREAM_PORTS } from "@/plugins/builtin/runtime/public/ports";

const HooksPane = lazy(() =>
  import("./ui/HooksPane").then(({ HooksPane }) => ({ default: HooksPane })),
);

export default definePlugin({
  name: "flame.builtin.hooks-pane",
  requires: { runtime: RUNTIME_STREAM_PORTS },
  setup(ctx) {
    const gateway = installHookTrustGateway();
    let connectionGeneration = ctx.runtime.connectionGeneration();
    const unsubscribeRuntime = ctx.runtime.subscribeConnection(() => {
      const next = ctx.runtime.connectionGeneration();
      if (next === connectionGeneration) return;
      connectionGeneration = next;
      gateway.replaceRuntimeGeneration();
    });
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

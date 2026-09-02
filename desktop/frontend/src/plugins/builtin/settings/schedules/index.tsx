import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../kit";
import { SCHEDULES_PANE } from "../kit/panes";
import {
  installScheduleGateway,
  registerScheduleDataProvider,
} from "./adapters/runtimeScheduleGateway";
import { RUNTIME_STREAM, followRuntimeGeneration } from "@/plugins/builtin/runtime/public/services";

const SchedulesPane = lazy(() =>
  import("./ui/SchedulesPane").then(({ SchedulesPane }) => ({ default: SchedulesPane })),
);

export default definePlugin({
  name: "flame.builtin.schedules-pane",
  requires: { runtime: RUNTIME_STREAM },
  setup(ctx) {
    const gateway = installScheduleGateway();
    registerScheduleDataProvider(ctx);
    const unsubscribeRuntime = followRuntimeGeneration(ctx.runtime, () =>
      gateway.replaceRuntimeGeneration(),
    );
    registerSettingsPane(ctx, {
      id: SCHEDULES_PANE,
      label: "settings.pane.schedules",
      group: "agent",
      icon: "clock",
      order: 58,
      component: SchedulesPane,
    });
    ctx.cleanup(() => {
      unsubscribeRuntime();
      gateway.dispose();
    });
  },
});

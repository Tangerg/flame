import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../kit";
import { SCHEDULES_PANE } from "../kit/panes";
import { installScheduleGateway } from "./adapters/runtimeScheduleGateway";
import {
  RUNTIME_STREAM_PORTS,
  followRuntimeGeneration,
} from "@/plugins/builtin/runtime/public/ports";

const SchedulesPane = lazy(() =>
  import("./ui/SchedulesPane").then(({ SchedulesPane }) => ({ default: SchedulesPane })),
);

export default definePlugin({
  name: "flame.builtin.schedules-pane",
  requires: { runtime: RUNTIME_STREAM_PORTS },
  setup(ctx) {
    const gateway = installScheduleGateway();
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

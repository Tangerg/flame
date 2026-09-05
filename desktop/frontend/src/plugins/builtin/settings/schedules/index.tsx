import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_STANDING_SURFACE } from "@/plugins/sdk/kernelPoints";
import { registerSettingsPane } from "../kit";
import { SCHEDULES_PANE } from "../kit/panes";
import {
  installScheduleGateway,
  registerScheduleDataProvider,
} from "./adapters/runtimeScheduleGateway";
import { RUNTIME_STREAM, followRuntimeGeneration } from "@/plugins/builtin/runtime/public/services";

/** The tools this pane answers for — see the note in plan-progress. */
export const SCHEDULE_STANDING_TOOLS = [
  "create_schedule",
  "list_schedules",
  "delete_schedule",
] as const;

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
    // The Schedules pane is where a schedule is read, created and deleted, so the transcript
    // row of a schedule tool is a second telling of what that pane already holds. Declared by
    // the plugin that owns the surface, the way Plan and Goal declare theirs.
    for (const key of SCHEDULE_STANDING_TOOLS) {
      ctx.contribute(TOOL_STANDING_SURFACE, SCHEDULES_PANE, { key });
    }
    ctx.cleanup(() => {
      unsubscribeRuntime();
      gateway.dispose();
    });
  },
});

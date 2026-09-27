import type { FlameClient } from "@flame/runtime-contract/client";
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

export const SCHEDULE_STANDING_TOOLS = [
  "create_schedule",
  "list_schedules",
  "delete_schedule",
] as const;

const SchedulesPane = lazy(() =>
  import("./ui/SchedulesPane").then(({ SchedulesPane }) => ({ default: SchedulesPane })),
);

export function createSchedulesPlugin(runtimeClient: () => FlameClient) {
  return definePlugin({
    name: "flame.builtin.schedules-pane",
    requires: { runtime: RUNTIME_STREAM },
    setup(ctx) {
      const gateway = installScheduleGateway(runtimeClient);
      ctx.cleanup(() => gateway.dispose());
      registerScheduleDataProvider(ctx, runtimeClient);
      ctx.cleanup(followRuntimeGeneration(ctx.runtime, () => gateway.replaceRuntimeGeneration()));
      registerSettingsPane(ctx, {
        id: SCHEDULES_PANE,
        label: "settings.pane.schedules",
        description: "schedules.intro",
        group: "agent",
        icon: "clock",
        order: 58,
        component: SchedulesPane,
      });
      for (const key of SCHEDULE_STANDING_TOOLS) {
        ctx.contribute(TOOL_STANDING_SURFACE, SCHEDULES_PANE, { key });
      }
    },
  });
}

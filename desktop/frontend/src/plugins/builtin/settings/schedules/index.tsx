import type { FlameClient } from "@flame/runtime-contract/client";
import { definePlugin } from "@/plugins/sdk";
import {
  installScheduleGateway,
  registerScheduleDataProvider,
} from "./adapters/runtimeScheduleGateway";
import { RUNTIME_STREAM, followRuntimeGeneration } from "@/plugins/builtin/runtime/public/services";

export function createSchedulesPlugin(runtimeClient: () => FlameClient) {
  return definePlugin({
    name: "flame.builtin.schedules",
    requires: { runtime: RUNTIME_STREAM },
    setup(ctx) {
      const gateway = installScheduleGateway(runtimeClient);
      ctx.cleanup(() => gateway.dispose());
      registerScheduleDataProvider(ctx, runtimeClient);
      ctx.cleanup(followRuntimeGeneration(ctx.runtime, () => gateway.replaceRuntimeGeneration()));
    },
  });
}

import { definePlugin } from "@/extensions/definePlugin";
import { initFrontendObservability } from "@/features/observability/frontendObservability";
import { startObservability } from "@/features/observability/observabilityLifecycle";
export function createObservabilityPlugin(service: { name: string; version: string }) {
  return definePlugin({
    name: "flame.builtin.observability",
    setup(ctx) {
      ctx.cleanup(
        startObservability(
          () => initFrontendObservability(service),
          (error) => {
            console.warn(
              "[observability] initialization failed; running without telemetry:",
              error,
            );
          },
        ),
      );
    },
  });
}

import { CONFIG } from "@/extensions/config";
import { definePlugin } from "@/extensions/definePlugin";
import { type createRuntimeConnection } from "@/features/connection/adapters/runtimeConnection";
import {
  RUNTIME_SERVER_SCOPE,
  RUNTIME_STREAM,
  type RuntimeConnectionGeneration,
} from "@/features/connection/public/services";
export function createRuntimePlugin(connection: ReturnType<typeof createRuntimeConnection>) {
  return definePlugin({
    name: "flame.builtin.runtime",
    provides: { serverScope: RUNTIME_SERVER_SCOPE, stream: RUNTIME_STREAM },
    requires: { config: CONFIG },
    setup(ctx) {
      const binding = connection.bind(ctx);
      ctx.cleanup(binding.dispose);
      return {
        serverScope: {
          subscribeReplacement: (onReplace: () => void) =>
            binding.owner.subscribeServerReplacement(onReplace),
        },
        stream: {
          connectionGeneration: () => binding.owner.connectionGeneration(),
          subscribeConnection: (onChange: () => void) =>
            binding.owner.subscribeConnection(onChange),
          reportConnectionLoss: (expectedGeneration: RuntimeConnectionGeneration) =>
            binding.owner.reportConnectionLoss(expectedGeneration),
        },
      };
    },
  });
}

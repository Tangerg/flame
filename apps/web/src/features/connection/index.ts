import { type FlameClient, type SidecarClient } from "@flame/runtime-client";
import { type RuntimeEndpointTarget } from "@/features/connection/application/ports/runtimeEndpoint";
import { CONFIG } from "@/extensions/config";
import { definePlugin } from "@/extensions/definePlugin";
import { bindRuntimeEndpointConfiguration } from "@/features/connection/adapters/runtimeEndpointConfiguration";
import { installRuntimeMutationJournalStorage } from "@/features/connection/adapters/runtimeMutationJournalStorage";
import { runtimeServiceInspector } from "@/features/connection/adapters/runtimeServiceInspector";
import { startRuntimeConnection } from "@/features/connection/adapters/runtimeConnectionProjection";
import {
  RUNTIME_SERVER_SCOPE,
  RUNTIME_STREAM,
  type RuntimeConnectionGeneration,
} from "@/features/connection/public/services";
export function createRuntimePlugin(
  runtimeClient: () => FlameClient,
  runtimeSidecar: () => SidecarClient,
  bootstrap: () => RuntimeEndpointTarget,
) {
  return definePlugin({
    name: "flame.builtin.runtime",
    provides: { serverScope: RUNTIME_SERVER_SCOPE, stream: RUNTIME_STREAM },
    requires: { config: CONFIG },
    setup(ctx) {
      let connection!: ReturnType<typeof startRuntimeConnection>;
      bindRuntimeEndpointConfiguration(
        ctx,
        (commit) => {
          void connection.replaceEndpoint(commit);
        },
        bootstrap(),
      );
      ctx.cleanup(installRuntimeMutationJournalStorage(ctx));
      connection = startRuntimeConnection(runtimeServiceInspector(runtimeClient, runtimeSidecar));
      ctx.cleanup(() => connection.dispose());
      return {
        serverScope: {
          subscribeReplacement: (onReplace: () => void) =>
            connection.subscribeServerReplacement(onReplace),
        },
        stream: {
          connectionGeneration: () => connection.connectionGeneration(),
          subscribeConnection: (onChange: () => void) => connection.subscribeConnection(onChange),
          reportConnectionLoss: (expectedGeneration: RuntimeConnectionGeneration) =>
            connection.reportConnectionLoss(expectedGeneration),
        },
      };
    },
  });
}

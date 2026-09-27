import type { FlameClient, SidecarClient } from "@flame/runtime-contract/client";
import type { RuntimeEndpointTarget } from "./application/ports/runtimeEndpoint";
import { CONFIG, definePlugin } from "@/plugins/sdk";
import { installRuntimeEndpointConfiguration } from "./adapters/runtimeEndpointConfiguration";
import { installRuntimeMutationJournalStorage } from "./adapters/runtimeMutationJournalStorage";
import { runtimeServiceInspector } from "./adapters/runtimeServiceInspector";
import { startRuntimeConnection } from "./adapters/runtimeConnectionProjection";
import {
  RUNTIME_SERVER_SCOPE,
  RUNTIME_STREAM,
  type RuntimeConnectionGeneration,
} from "@/plugins/builtin/runtime/public/services";

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
      const disposeEndpoint = installRuntimeEndpointConfiguration(
        ctx,
        (commit) => {
          void connection.replaceEndpoint(commit);
        },
        bootstrap(),
      );
      const disposeMutationJournal = installRuntimeMutationJournalStorage(ctx);
      connection = startRuntimeConnection(runtimeServiceInspector(runtimeClient, runtimeSidecar));
      ctx.cleanup(() => {
        connection.dispose();
        disposeMutationJournal();
        disposeEndpoint();
      });
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

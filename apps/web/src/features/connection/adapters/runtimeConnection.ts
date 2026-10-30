import { RpcCancellationError, type RequestMeta } from "@flame/runtime-client";
import { type ClientBootstrap, type ClientHost } from "@/platform/host";
import { tupleKey } from "@/lib/tupleKey";
import { type FlameClient, type SidecarClient } from "@flame/runtime-client";
import { type RuntimeMutationJournalStorage } from "@/features/connection/adapters/runtimeMutationJournalStorage";
import {
  createRuntimeEndpoint,
  type RuntimeEndpointConfiguration,
} from "@/features/connection/application/runtimeEndpoint";
import { bindRuntimeEndpointConfiguration } from "@/features/connection/adapters/runtimeEndpointConfiguration";
import { createRuntimeConnectionProjection } from "@/features/connection/adapters/runtimeConnectionProjection";
import { runtimeServiceInspector } from "@/features/connection/adapters/runtimeServiceInspector";
import { type RuntimeAccess } from "@/features/connection/public/runtime";

import { createHttpTransport, createFlameClient, createSidecarClient } from "@flame/runtime-client";
export function createRuntimeConnection(
  host: ClientHost,
  requestMeta: (surface: "desktop" | "web") => RequestMeta,
  storage: RuntimeMutationJournalStorage,
) {
  const projection = createRuntimeConnectionProjection();
  let configuration: RuntimeEndpointConfiguration | null = null;
  const endpoint = createRuntimeEndpoint({
    read: () => configuration?.read() ?? bootstrap().runtime,
    defaultTarget: () => configuration?.defaultTarget() ?? bootstrap().runtime,
    replace: (target) => {
      assertOpen();
      if (!configuration) throw new Error("Runtime endpoint is not ready");
      return configuration.replace(target);
    },
    reset: () => {
      assertOpen();
      if (!configuration) throw new Error("Runtime endpoint is not ready");
      return configuration.reset();
    },
  });
  const access: RuntimeAccess = {
    capabilities: projection.capabilities,
    status: projection.status,
    endpoint,
  };
  let shared: {
    signature: string;
    client: FlameClient;
  } | null = null;
  let sidecar: { endpoint: string; client: SidecarClient } | null = null;
  const retiring = new Set<Promise<void>>();
  const retirementFailures: unknown[] = [];
  let hostBootstrap: ClientBootstrap | null = null;
  let bootstrapLease: object = {};
  let closed = false;
  let disposal: Promise<void> | undefined;
  const assertOpen = () => {
    if (closed) throw new Error("Runtime connection is closed");
  };
  const retire = (client: FlameClient) => {
    let closing!: Promise<void>;
    closing = client
      .close()
      .catch((error: unknown) => {
        retirementFailures.push(error);
      })
      .finally(() => retiring.delete(closing));
    retiring.add(closing);
  };
  const bootstrap = (): ClientBootstrap => {
    assertOpen();
    if (!hostBootstrap) throw new Error("client host has not completed bootstrap");
    return hostBootstrap;
  };
  const readHost = async (signal?: AbortSignal) => {
    const lease = (bootstrapLease = {});
    const observation = await host.bootstrap();
    if (closed || lease !== bootstrapLease || signal?.aborted) return false;
    hostBootstrap = observation;
    return true;
  };
  const target = () => configuration?.read() ?? bootstrap().runtime;
  const client = () => {
    assertOpen();
    const { endpoint: baseUrl, localToken } = target();
    const signature = tupleKey(baseUrl, localToken ?? "");
    if (shared?.signature === signature) return shared.client;
    if (shared) retire(shared.client);
    const client = createFlameClient(createHttpTransport({ baseUrl, localToken }), {
      requestMeta: () => requestMeta(host.kind),
      capabilities: projection.capabilities.negotiated,
      mutationStorage: storage,
    });
    shared = { signature, client };
    return client;
  };
  const sidecarClient = () => {
    assertOpen();
    const { endpoint } = target();
    if (sidecar?.endpoint === endpoint) return sidecar.client;
    const client = createSidecarClient({ baseUrl: endpoint });
    sidecar = { endpoint, client };
    return client;
  };
  return {
    client,
    sidecar: sidecarClient,
    access,
    stream: projection.stream,
    scope: projection.scope,
    bind(ctx: Parameters<typeof bindRuntimeEndpointConfiguration>[0]) {
      assertOpen();
      const binding = bindRuntimeEndpointConfiguration(
        ctx,
        (commit) => {
          void owner.replaceEndpoint(commit);
        },
        () => bootstrap().runtime,
      );
      configuration = binding;
      const inspector = runtimeServiceInspector(client, sidecarClient);
      const owner = projection.start({
        async inspect(signal) {
          if (binding.usesHostCredential() && !(await readHost(signal))) {
            throw new RpcCancellationError();
          }
          return inspector.inspect(signal);
        },
      });
      return {
        owner,
        dispose() {
          owner.dispose();
          if (configuration === binding) configuration = null;
        },
      };
    },
    endpoint: () => {
      assertOpen();
      return target().endpoint;
    },
    bootstrap,
    localWorkspaceAvailable: () => {
      const local = hostBootstrap?.localFilesystemEndpoint;
      return local != null && local.replace(/\/+$/, "") === target().endpoint.replace(/\/+$/, "");
    },
    async initialize() {
      assertOpen();
      hostBootstrap = null;
      await readHost();
    },
    dispose() {
      if (disposal) return disposal;
      closed = true;
      projection.dispose();
      configuration = null;
      bootstrapLease = {};
      hostBootstrap = null;
      if (shared) retire(shared.client);
      shared = null;
      sidecar = null;
      disposal = Promise.all(retiring).then(() => {
        if (retirementFailures.length) {
          throw new AggregateError(retirementFailures, "Runtime connection cleanup failed");
        }
      });
      return disposal;
    },
  };
}

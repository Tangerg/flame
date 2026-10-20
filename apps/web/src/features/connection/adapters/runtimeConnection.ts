import { type RequestMeta } from "@flame/runtime-client";
import { negotiatedCapabilities } from "@/features/connection/public/capabilities";
import { configuredRuntimeTarget } from "@/features/connection/public/endpoint";
import { type ClientBootstrap, type ClientHost } from "@/platform/host";
import { installedRuntimeMutationJournalStorage } from "@/features/connection/public/mutationJournal";
import { tupleKey } from "@/lib/tupleKey";
import { type FlameClient, type SidecarClient } from "@flame/runtime-client";
import { type RuntimeMutationJournalStorage } from "@/features/connection/public/mutationJournal";
import {
  createHttpTransport,
  createFlameClient,
  createMutationJournal,
  createSidecarClient,
} from "@flame/runtime-client";
export function createRuntimeConnection(
  host: ClientHost,
  requestMeta: (surface: "desktop" | "web") => RequestMeta,
) {
  let shared: {
    signature: string;
    storage: RuntimeMutationJournalStorage | null;
    client: FlameClient;
  } | null = null;
  let sidecar: { endpoint: string; client: SidecarClient } | null = null;
  const retiring = new Set<Promise<void>>();
  const retirementFailures: unknown[] = [];
  let hostBootstrap: (ClientBootstrap & { kind: ClientHost["kind"] }) | null = null;
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
  const target = () => configuredRuntimeTarget() ?? bootstrap().runtime;
  return {
    client: () => {
      assertOpen();
      const { endpoint: baseUrl, localToken } = target();
      const signature = tupleKey(baseUrl, localToken ?? "");
      const storage = installedRuntimeMutationJournalStorage();
      if (shared?.signature === signature && shared.storage === storage) return shared.client;
      if (shared) retire(shared.client);
      const client = createFlameClient(createHttpTransport({ baseUrl, localToken }), {
        requestMeta: () => requestMeta(hostBootstrap?.kind ?? host.kind),
        capabilities: negotiatedCapabilities,
        mutationJournal: storage
          ? createMutationJournal({
              storage,
              scope: () => {
                const idempotency = negotiatedCapabilities()?.limits.idempotency;
                return idempotency
                  ? {
                      namespace: idempotency.namespace,
                      retentionSeconds: idempotency.retentionSeconds,
                    }
                  : null;
              },
            })
          : undefined,
      });
      shared = { signature, storage, client };
      return client;
    },
    sidecar: () => {
      assertOpen();
      const { endpoint } = target();
      if (sidecar?.endpoint === endpoint) return sidecar.client;
      const client = createSidecarClient({ baseUrl: endpoint });
      sidecar = { endpoint, client };
      return client;
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
      const lease = (bootstrapLease = {});
      hostBootstrap = null;
      const bootstrap = await host.bootstrap();
      if (closed || lease !== bootstrapLease) return;
      hostBootstrap = { ...bootstrap, kind: host.kind };
    },
    dispose() {
      if (disposal) return disposal;
      closed = true;
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

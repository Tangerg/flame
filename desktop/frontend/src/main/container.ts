import { runtimeRequestMeta } from "@/main/runtimeProtocol";
import { negotiatedCapabilities } from "@/plugins/builtin/runtime/public/capabilities";
import { configuredRuntimeTarget } from "@/plugins/builtin/runtime/public/endpoint";
import { createClientHost } from "@/platform/clientHost";
import type { ClientBootstrap, ClientHost } from "@/platform/host";
import { installedRuntimeMutationJournalStorage } from "@/plugins/builtin/runtime/public/mutationJournal";
import { tupleKey } from "@/lib/tupleKey";
import type { FlameClient, SidecarClient } from "@flame/runtime-contract/client";
import type { RuntimeMutationJournalStorage } from "@/plugins/builtin/runtime/public/mutationJournal";
import {
  createHttpTransport,
  createFlameClient,
  createMutationJournal,
  createSidecarClient,
} from "@flame/runtime-contract/client";

export interface Container {
  client: () => FlameClient;
  sidecar: () => SidecarClient;
  host: ClientHost;
  bootstrap(): ClientBootstrap;
  localWorkspaceAvailable(): boolean;
}

interface DefaultContainerOwner {
  readonly container: Container;
  initializeClientHost(host: ClientHost): Promise<void>;
  replaceClientHost(): void;
  dispose(): Promise<void>;
}

function defaultContainer(): DefaultContainerOwner {
  let shared: {
    signature: string;
    storage: RuntimeMutationJournalStorage | null;
    client: FlameClient;
  } | null = null;
  let sidecar: { endpoint: string; client: SidecarClient } | null = null;
  const retiring = new Set<Promise<void>>();
  let hostBootstrap: (ClientBootstrap & { kind: ClientHost["kind"] }) | null = null;
  let bootstrapLease: object = {};
  let closed = false;
  let disposal: Promise<void> | undefined;
  const assertOpen = () => {
    if (closed) throw new Error("Client container is closed");
  };
  const retire = (client: FlameClient) => {
    let closing!: Promise<void>;
    closing = client
      .close()
      .catch(() => undefined)
      .finally(() => retiring.delete(closing));
    retiring.add(closing);
  };
  const bootstrap = (): ClientBootstrap => {
    assertOpen();
    if (!hostBootstrap) throw new Error("client host has not completed bootstrap");
    return hostBootstrap;
  };
  const target = () => configuredRuntimeTarget() ?? bootstrap().runtime;
  const container: Container = {
    client: () => {
      assertOpen();
      const { endpoint: baseUrl, localToken } = target();
      const signature = tupleKey(baseUrl, localToken ?? "");
      const storage = installedRuntimeMutationJournalStorage();
      if (shared?.signature === signature && shared.storage === storage) return shared.client;
      if (shared) retire(shared.client);
      const client = createFlameClient(createHttpTransport({ baseUrl, localToken }), {
        requestMeta: () => runtimeRequestMeta(hostBootstrap?.kind ?? container.host.kind),
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
    host: createClientHost(),
    bootstrap,
    localWorkspaceAvailable: () => {
      const local = hostBootstrap?.localFilesystemEndpoint;
      return local != null && local.replace(/\/+$/, "") === target().endpoint.replace(/\/+$/, "");
    },
  };
  return {
    container,
    async initializeClientHost(host) {
      assertOpen();
      const lease = (bootstrapLease = {});
      hostBootstrap = null;
      const bootstrap = await host.bootstrap();
      if (closed || lease !== bootstrapLease) return;
      hostBootstrap = { ...bootstrap, kind: host.kind };
    },
    replaceClientHost() {
      assertOpen();
      bootstrapLease = {};
      hostBootstrap = null;
    },
    dispose() {
      if (disposal) return disposal;
      closed = true;
      bootstrapLease = {};
      hostBootstrap = null;
      if (shared) retire(shared.client);
      shared = null;
      sidecar = null;
      disposal = Promise.all(retiring).then(() => undefined);
      return disposal;
    },
  };
}

let defaultOwner = defaultContainer();
let instance: Container = defaultOwner.container;

export function getContainer(): Container {
  return instance;
}

export function setContainer(next: Partial<Container>): void {
  if (next.host) defaultOwner.replaceClientHost();
  instance = { ...instance, ...next };
}

export async function initializeClientHost(): Promise<void> {
  const owner = defaultOwner;
  const host = instance.host;
  await owner.initializeClientHost(host);
}

export function disposeContainer(): Promise<void> {
  return defaultOwner.dispose();
}

export async function resetContainer(): Promise<void> {
  const retired = defaultOwner;
  defaultOwner = defaultContainer();
  instance = defaultOwner.container;
  await retired.dispose();
}

import { runtimeRequestMeta } from "@/main/runtimeProtocol";
import { negotiatedCapabilities } from "@/plugins/builtin/runtime/public/capabilities";
import { currentRuntimeEndpoint } from "@/plugins/builtin/runtime/public/endpoint";
import { installedRuntimeMutationJournalStorage } from "@/plugins/builtin/runtime/public/mutationJournal";
import { tupleKey } from "@/lib/tupleKey";
import type { DesktopBootstrap, DesktopHostClient, FlameClient, SidecarClient } from "@/rpc";
import type { RuntimeMutationJournalStorage } from "@/plugins/builtin/runtime/public/mutationJournal";
import {
  createDesktopHostClient,
  createHttpTransport,
  createFlameClient,
  createMutationJournal,
  createSidecarClient,
} from "@/rpc";

export interface Container {
  client: () => FlameClient;
  sidecar: () => SidecarClient;
  desktop: DesktopHostClient;
}

interface DefaultContainerOwner {
  readonly container: Container;
  initializeDesktopHost(desktop: DesktopHostClient): Promise<void>;
  replaceDesktopHost(): void;
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
  let desktopBootstrap: DesktopBootstrap | null = null;
  let bootstrapLease: object = {};
  let closed = false;
  let disposal: Promise<void> | undefined;
  const assertOpen = () => {
    if (closed) throw new Error("Desktop container is closed");
  };
  const retire = (client: FlameClient) => {
    let closing!: Promise<void>;
    closing = client
      .close()
      .catch(() => undefined)
      .finally(() => retiring.delete(closing));
    retiring.add(closing);
  };
  const localTokenFor = (endpoint: string): string | undefined => {
    const local = desktopBootstrap?.localRuntime;
    if (!local) return undefined;
    const normalized = endpoint.replace(/\/+$/, "");
    return normalized === local.endpoint.replace(/\/+$/, "") ? local.localToken : undefined;
  };
  const container: Container = {
    client: () => {
      assertOpen();
      const baseUrl = currentRuntimeEndpoint();
      const localToken = localTokenFor(baseUrl);
      const signature = tupleKey(baseUrl, localToken ?? "");
      const storage = installedRuntimeMutationJournalStorage();
      if (shared?.signature === signature && shared.storage === storage) return shared.client;
      if (shared) retire(shared.client);
      const client = createFlameClient(createHttpTransport({ baseUrl, localToken }), {
        requestMeta: runtimeRequestMeta,
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
      const endpoint = currentRuntimeEndpoint();
      if (sidecar?.endpoint === endpoint) return sidecar.client;
      const client = createSidecarClient({ baseUrl: endpoint });
      sidecar = { endpoint, client };
      return client;
    },
    desktop: createDesktopHostClient(),
  };
  return {
    container,
    async initializeDesktopHost(desktop) {
      assertOpen();
      const lease = (bootstrapLease = {});
      desktopBootstrap = null;
      const bootstrap = await desktop.bootstrap();
      if (closed || lease !== bootstrapLease) return;
      desktopBootstrap = bootstrap;
    },
    replaceDesktopHost() {
      assertOpen();
      bootstrapLease = {};
      desktopBootstrap = null;
    },
    dispose() {
      if (disposal) return disposal;
      closed = true;
      bootstrapLease = {};
      desktopBootstrap = null;
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
  if (next.desktop) defaultOwner.replaceDesktopHost();
  instance = { ...instance, ...next };
}

export async function initializeDesktopHost(): Promise<void> {
  const owner = defaultOwner;
  const desktop = instance.desktop;
  await owner.initializeDesktopHost(desktop);
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

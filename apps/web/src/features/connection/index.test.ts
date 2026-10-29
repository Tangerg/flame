import { IMAGE_FILENAME_STEM } from "@/app/product";
import { runtimeRequestMeta } from "@/app/runtimeProtocol";
import { createBrowserHost } from "@/platform/browserHost";
import { createStorage } from "@/extensions/storage";
import { createRuntimeMutationJournalStorage } from "@/features/connection/adapters/runtimeMutationJournalStorage";
import { createRuntimeConnection } from "@/features/connection/adapters/runtimeConnection";
let connection: ReturnType<typeof createRuntimeConnection>;
import { shellServices } from "@/app/shellServices";
import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import * as runtimeSdk from "@flame/runtime-client";
import {
  HTTP_ENDPOINTS,
  PROTOCOL_VERSION,
  type DiscoverResponse,
  type FlameClient,
  type Methods,
  type ReadinessStatus,
  type SidecarClient,
} from "@flame/runtime-client";

import { createRuntimePlugin } from "@/features/connection/index";
import { startKernel, stopKernel } from "@/extensions/bootstrap";
import { loadPluginsForTest, resetKernelForTest } from "@/test/kernel";
let runtimeClient: () => FlameClient = () => {
  throw new Error("Runtime test client is not configured");
};

const getRuntimeClient = () => runtimeClient();

let runtimeSidecar: () => SidecarClient = () => {
  throw new Error("Runtime test sidecar is not configured");
};

const getRuntimeSidecar = () => runtimeSidecar();
const createFlameClient = runtimeSdk.createFlameClient;
let readClientCapabilities:
  NonNullable<Parameters<typeof createFlameClient>[1]>["capabilities"] | undefined;

function negotiatedCapabilities() {
  if (!readClientCapabilities) throw new Error("Runtime client capabilities are not configured");
  return readClientCapabilities();
}

const discovery: DiscoverResponse = {
  protocolVersion: PROTOCOL_VERSION,
  serverInfo: {
    instanceId: "runtime_1",
    name: "flame-runtime",
    version: "1.2.3",
    defaultWorkspace: { path: "/w" },
    home: "/h",
  },
  capabilities: {
    features: {},
    runEvents: [],
    runtimeTopics: [],
    streamingMethods: [],
    limits: {
      idempotency: { namespace: "idp_00000000000000000000000000000001", retentionSeconds: 86_400 },
      runReplay: { scope: "runtimeInstanceRootSegment", maxEvents: 2048, maxBytes: 16_777_216 },
      mcpAuthorizationAttempts: { retentionSeconds: 600 },
      runtimeSubscription: {
        maxTopics: 8,
        maxWatches: 8,
        maxPaths: 256,
        maxDirectoryEntries: 10000,
        maxFileBytes: 1048576,
      },
    },
  },
};

function healthySidecar(): SidecarClient {
  return {
    info: vi.fn().mockResolvedValue({
      protocolVersion: PROTOCOL_VERSION,
      server: { name: "flame-runtime", version: "1.2.3", instanceId: "runtime_1" },
      transport: "http",
      endpoints: {
        rpc: HTTP_ENDPOINTS.rpc.path,
        info: HTTP_ENDPOINTS.info.path,
        liveness: HTTP_ENDPOINTS.liveness.path,
        readiness: HTTP_ENDPOINTS.readiness.path,
      },
    }),
    liveness: vi.fn().mockResolvedValue({ status: "ok", instanceId: "runtime_1" }),
    readiness: vi.fn().mockResolvedValue({ status: "ok", instanceId: "runtime_1" }),
  };
}

function stubRuntime(
  discover: Methods["runtime"]["discover"],
  sidecar: SidecarClient = healthySidecar(),
) {
  ({ client: runtimeClient, sidecar: runtimeSidecar } = {
    client: () =>
      ({
        runtime: { discover },
      }) as unknown as FlameClient,
    sidecar: () => sidecar,
  });
}

beforeEach(async () => {
  readClientCapabilities = undefined;
  vi.spyOn(runtimeSdk, "createFlameClient").mockImplementation((transport, options) => {
    readClientCapabilities = options?.capabilities;
    return createFlameClient(transport, options);
  });
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = new URL(
      typeof input === "string" ? input : input instanceof URL ? input.href : input.url,
    );
    let value: unknown;
    if (url.pathname === HTTP_ENDPOINTS.rpc.path) {
      const request = JSON.parse(String(init?.body)) as { id: unknown; method: string };
      if (request.method !== "runtime.discover")
        throw new Error(`Unexpected test method ${request.method}`);
      value = {
        jsonrpc: "2.0",
        id: request.id,
        result: await getRuntimeClient().runtime.discover(),
      };
    } else if (url.pathname === HTTP_ENDPOINTS.info.path) value = await getRuntimeSidecar().info();
    else if (url.pathname === HTTP_ENDPOINTS.liveness.path)
      value = await getRuntimeSidecar().liveness();
    else if (url.pathname === HTTP_ENDPOINTS.readiness.path)
      value = await getRuntimeSidecar().readiness();
    else throw new Error(`Unexpected test endpoint ${url.pathname}`);
    return new Response(JSON.stringify(value), { headers: { "Content-Type": "application/json" } });
  });
  connection = createRuntimeConnection(
    createBrowserHost(IMAGE_FILENAME_STEM),
    runtimeRequestMeta,
    createRuntimeMutationJournalStorage(createStorage("test.runtime")),
  );
  await connection.initialize();
});

afterEach(async () => {
  await resetKernelForTest();

  await connection.dispose();
  vi.restoreAllMocks();
});

describe("runtime plugin", () => {
  it("discovers capabilities through the supervised Runtime connection", async () => {
    const discover = vi.fn().mockResolvedValue(discovery);
    stubRuntime(discover);

    await loadPluginsForTest(createRuntimePlugin(connection));

    await vi.waitFor(() => {
      expect(negotiatedCapabilities()).not.toBeNull();
    });
    expect(discover).toHaveBeenCalledOnce();
  });

  it("inspects all operational endpoints through the Runtime context", async () => {
    const sidecar = healthySidecar();
    stubRuntime(vi.fn().mockResolvedValue(discovery), sidecar);

    await loadPluginsForTest(createRuntimePlugin(connection));

    await vi.waitFor(() => {
      expect(connection.access.status.snapshot().phase).toBe("ready");
    });
    expect(sidecar.info).toHaveBeenCalledOnce();
    expect(sidecar.liveness).toHaveBeenCalledOnce();
    expect(sidecar.readiness).toHaveBeenCalledOnce();
  });

  it("publishes sidecar failure without preserving a stale ready phase", async () => {
    const sidecar = healthySidecar();
    sidecar.readiness = vi.fn().mockRejectedValue(new Error("connection refused"));
    stubRuntime(vi.fn().mockResolvedValue(discovery), sidecar);

    await loadPluginsForTest(createRuntimePlugin(connection));

    await vi.waitFor(() => {
      expect(connection.access.status.snapshot()).toMatchObject({
        phase: "unavailable",
        failure: {
          reason: "failed",
          detail: `sidecar ${HTTP_ENDPOINTS.readiness.path}: connection refused`,
        },
      });
    });
  });

  it("degrades without publishing stale capabilities when discovery fails", async () => {
    stubRuntime(
      vi.fn().mockResolvedValueOnce(discovery).mockRejectedValue(new Error("method not found")),
    );

    await loadPluginsForTest(createRuntimePlugin(connection));
    await vi.waitFor(() => expect(negotiatedCapabilities()).not.toBeNull());
    await connection.access.status.refresh();
    await vi.waitFor(() => {
      expect(connection.access.status.snapshot()).toMatchObject({
        phase: "unavailable",
        failure: { reason: "failed", detail: "fetch failed: method not found" },
      });
    });
    expect(negotiatedCapabilities()).toBeNull();
  });

  it("does not publish a discovery result after the plugin is unloaded", async () => {
    let resolveDiscovery: (value: DiscoverResponse) => void = () => undefined;
    const discover = vi.fn(
      () =>
        new Promise<DiscoverResponse>((resolve) => {
          resolveDiscovery = resolve;
        }),
    );
    stubRuntime(discover);

    await loadPluginsForTest(createRuntimePlugin(connection));
    await vi.waitFor(() => expect(discover).toHaveBeenCalledOnce());
    await resetKernelForTest();

    resolveDiscovery(discovery);
    await Promise.resolve();
    await Promise.resolve();

    expect(negotiatedCapabilities()).toBeNull();
  });

  it("automatically rediscovers and republishes capabilities after a cold-start outage", async () => {
    vi.useFakeTimers();
    try {
      const discover = vi
        .fn()
        .mockRejectedValueOnce(new Error("offline"))
        .mockResolvedValue(discovery);
      stubRuntime(discover);

      await loadPluginsForTest(createRuntimePlugin(connection));
      await vi.advanceTimersByTimeAsync(0);
      expect(negotiatedCapabilities()).toBeNull();
      expect(connection.access.status.snapshot().phase).toBe("unavailable");

      await vi.advanceTimersByTimeAsync(1_000);
      expect(discover).toHaveBeenCalledTimes(2);
      expect(negotiatedCapabilities()).toEqual(discovery.capabilities);
      expect(connection.access.status.snapshot().phase).toBe("ready");
    } finally {
      vi.useRealTimers();
    }
  });

  it("does not publish a sidecar result after the plugin is unloaded", async () => {
    let resolveReadiness: (value: ReadinessStatus) => void = () => undefined;
    const sidecar = healthySidecar();
    sidecar.readiness = vi.fn<SidecarClient["readiness"]>(
      () =>
        new Promise<ReadinessStatus>((resolve) => {
          resolveReadiness = resolve;
        }),
    );
    stubRuntime(vi.fn().mockResolvedValue(discovery), sidecar);

    await loadPluginsForTest(createRuntimePlugin(connection));
    await vi.waitFor(() => expect(sidecar.readiness).toHaveBeenCalledOnce());
    await resetKernelForTest();

    resolveReadiness({ status: "ok", instanceId: "runtime_1" });
    await Promise.resolve();
    await Promise.resolve();

    expect(connection.access.status.snapshot()).toEqual({
      phase: "checking",
      observation: null,
      failure: null,
    });
  });

  it("does not let a retired Runtime installation clear its successor projection", async () => {
    stubRuntime(vi.fn().mockResolvedValue(discovery));
    const retired = await startKernel([shellServices, createRuntimePlugin(connection)]);
    let successor: Awaited<ReturnType<typeof startKernel>> | undefined;
    try {
      await vi.waitFor(() => {
        expect(connection.access.status.snapshot().phase).toBe("ready");
      });

      stubRuntime(vi.fn().mockResolvedValue(discovery));
      successor = await startKernel([shellServices, createRuntimePlugin(connection)]);
      await vi.waitFor(() => {
        expect(connection.access.status.snapshot().phase).toBe("ready");
        expect(negotiatedCapabilities()).toEqual(discovery.capabilities);
      });

      await stopKernel(retired);

      expect(connection.access.status.snapshot().phase).toBe("ready");
      expect(negotiatedCapabilities()).toEqual(discovery.capabilities);
    } finally {
      if (successor) await stopKernel(successor);
      else await stopKernel(retired);
    }
  });
});

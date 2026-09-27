import { afterEach, describe, expect, it, vi } from "vitest";
import {
  HTTP_ENDPOINTS,
  PROTOCOL_VERSION,
  type DiscoverResponse,
  type FlameClient,
  type Methods,
  type ReadinessStatus,
  type SidecarClient,
} from "@flame/runtime-contract/client";
import {
  resetRuntimeConnectionForTest,
  useRuntimeConnectionStore,
} from "./adapters/runtimeConnectionProjection";
import { createRuntimePlugin } from "./index";
import { startKernel, stopKernel } from "@/plugins/sdk/bootstrap";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";

let runtimeClient: () => FlameClient = () => {
  throw new Error("Runtime test client is not configured");
};
const getRuntimeClient = () => runtimeClient();
let runtimeSidecar: () => SidecarClient = () => {
  throw new Error("Runtime test sidecar is not configured");
};
const getRuntimeSidecar = () => runtimeSidecar();

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
      idempotency: { namespace: "idp_test", retentionSeconds: 86_400 },
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

afterEach(async () => {
  await resetKernelForTest();

  resetRuntimeConnectionForTest();
  vi.restoreAllMocks();
});

describe("runtime plugin", () => {
  it("discovers capabilities through the supervised Runtime connection", async () => {
    const discover = vi.fn().mockResolvedValue(discovery);
    stubRuntime(discover);

    await loadPluginsForTest(
      createRuntimePlugin(getRuntimeClient, getRuntimeSidecar, () => ({
        endpoint: window.location.origin,
      })),
    );

    await vi.waitFor(() => {
      expect(useRuntimeConnectionStore.getState().capabilities).not.toBeNull();
    });
    expect(discover).toHaveBeenCalledOnce();
  });

  it("inspects all operational endpoints through the Runtime context", async () => {
    const sidecar = healthySidecar();
    stubRuntime(vi.fn().mockResolvedValue(discovery), sidecar);

    await loadPluginsForTest(
      createRuntimePlugin(getRuntimeClient, getRuntimeSidecar, () => ({
        endpoint: window.location.origin,
      })),
    );

    await vi.waitFor(() => {
      expect(useRuntimeConnectionStore.getState().service.phase).toBe("ready");
    });
    expect(sidecar.info).toHaveBeenCalledOnce();
    expect(sidecar.liveness).toHaveBeenCalledOnce();
    expect(sidecar.readiness).toHaveBeenCalledOnce();
  });

  it("publishes sidecar failure without preserving a stale ready phase", async () => {
    const sidecar = healthySidecar();
    sidecar.readiness = vi.fn().mockRejectedValue(new Error("connection refused"));
    stubRuntime(vi.fn().mockResolvedValue(discovery), sidecar);

    await loadPluginsForTest(
      createRuntimePlugin(getRuntimeClient, getRuntimeSidecar, () => ({
        endpoint: window.location.origin,
      })),
    );

    await vi.waitFor(() => {
      expect(useRuntimeConnectionStore.getState().service).toMatchObject({
        phase: "unavailable",
        failure: { reason: "failed", detail: "connection refused" },
      });
    });
  });

  it("degrades without publishing stale capabilities when discovery fails", async () => {
    useRuntimeConnectionStore.setState({ capabilities: discovery.capabilities });
    stubRuntime(vi.fn().mockRejectedValue(new Error("method not found")));

    await loadPluginsForTest(
      createRuntimePlugin(getRuntimeClient, getRuntimeSidecar, () => ({
        endpoint: window.location.origin,
      })),
    );

    await vi.waitFor(() => {
      expect(useRuntimeConnectionStore.getState().service).toMatchObject({
        phase: "unavailable",
        failure: { reason: "failed", detail: "method not found" },
      });
    });
    expect(useRuntimeConnectionStore.getState().capabilities).toBeNull();
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

    await loadPluginsForTest(
      createRuntimePlugin(getRuntimeClient, getRuntimeSidecar, () => ({
        endpoint: window.location.origin,
      })),
    );
    await vi.waitFor(() => expect(discover).toHaveBeenCalledOnce());
    await resetKernelForTest();

    resolveDiscovery(discovery);
    await Promise.resolve();
    await Promise.resolve();

    expect(useRuntimeConnectionStore.getState().capabilities).toBeNull();
  });

  it("automatically rediscovers and republishes capabilities after a cold-start outage", async () => {
    vi.useFakeTimers();
    try {
      const discover = vi
        .fn()
        .mockRejectedValueOnce(new Error("offline"))
        .mockResolvedValue(discovery);
      stubRuntime(discover);

      await loadPluginsForTest(
        createRuntimePlugin(getRuntimeClient, getRuntimeSidecar, () => ({
          endpoint: window.location.origin,
        })),
      );
      await vi.advanceTimersByTimeAsync(0);
      expect(useRuntimeConnectionStore.getState().capabilities).toBeNull();
      expect(useRuntimeConnectionStore.getState().service.phase).toBe("unavailable");

      await vi.advanceTimersByTimeAsync(1_000);
      expect(discover).toHaveBeenCalledTimes(2);
      expect(useRuntimeConnectionStore.getState().capabilities).toEqual(discovery.capabilities);
      expect(useRuntimeConnectionStore.getState().service.phase).toBe("ready");
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

    await loadPluginsForTest(
      createRuntimePlugin(getRuntimeClient, getRuntimeSidecar, () => ({
        endpoint: window.location.origin,
      })),
    );
    await vi.waitFor(() => expect(sidecar.readiness).toHaveBeenCalledOnce());
    await resetKernelForTest();

    resolveReadiness({ status: "ok", instanceId: "runtime_1" });
    await Promise.resolve();
    await Promise.resolve();

    expect(useRuntimeConnectionStore.getState().service).toEqual({
      phase: "checking",
      observation: null,
      failure: null,
    });
  });

  it("does not let a retired Runtime installation clear its successor projection", async () => {
    stubRuntime(vi.fn().mockResolvedValue(discovery));
    const retired = await startKernel([
      createRuntimePlugin(getRuntimeClient, getRuntimeSidecar, () => ({
        endpoint: window.location.origin,
      })),
    ]);
    let successor: Awaited<ReturnType<typeof startKernel>> | undefined;
    try {
      await vi.waitFor(() => {
        expect(useRuntimeConnectionStore.getState().service.phase).toBe("ready");
      });

      stubRuntime(vi.fn().mockResolvedValue(discovery));
      successor = await startKernel([
        createRuntimePlugin(getRuntimeClient, getRuntimeSidecar, () => ({
          endpoint: window.location.origin,
        })),
      ]);
      await vi.waitFor(() => {
        expect(useRuntimeConnectionStore.getState().service.phase).toBe("ready");
        expect(useRuntimeConnectionStore.getState().capabilities).toEqual(discovery.capabilities);
      });

      await stopKernel(retired);

      expect(useRuntimeConnectionStore.getState().service.phase).toBe("ready");
      expect(useRuntimeConnectionStore.getState().capabilities).toEqual(discovery.capabilities);
    } finally {
      if (successor) await stopKernel(successor);
      else await stopKernel(retired);
    }
  });
});

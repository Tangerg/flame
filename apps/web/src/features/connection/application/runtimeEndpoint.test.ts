import { runtimeRequestMeta } from "@/app/runtimeProtocol";
import { createBrowserHost } from "@/platform/browserHost";
import { createDesktopHostClient } from "@/platform/desktopHost";
import { DesktopMethods } from "@/platform/desktop.generated";
import { RUNTIME_SERVICE_RETRY_BASE_MS } from "@/features/connection/application/runtimeService";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createRuntimeConnection } from "@/features/connection/adapters/runtimeConnection";
import { Configuration } from "@/extensions/config";
let config: Configuration;
import { type ConfigService } from "@/extensions/config";
import { type KeyValueStore } from "@/extensions/storage";
import {
  createHost,
  definePlugin,
  type Host,
  type LifetimeContext,
  type LifetimeOperations,
} from "dougong";
import {
  createRuntimeEndpoint,
  type RuntimeEndpoint,
  type RuntimeEndpointConfiguration,
} from "@/features/connection/application/runtimeEndpoint";
import { createRuntimeMutationJournalStorage } from "@/features/connection/adapters/runtimeMutationJournalStorage";
import { createStorage } from "@/extensions/storage";
import { bindRuntimeEndpointConfiguration } from "@/features/connection/adapters/runtimeEndpointConfiguration";
const DEFAULT_RUNTIME_ENDPOINT = "https://flame.example";

let endpointHost: Host;

let endpointLifetime: LifetimeContext;

function connectionHost(initial?: unknown): {
  host: { config: ConfigService; storage: KeyValueStore } & Pick<LifetimeOperations, "cleanup">;
  stored: Map<string, unknown>;
  dispose(): Promise<void>;
} {
  const owner = endpointLifetime.lifetime("endpoint");
  const stored = new Map<string, unknown>();
  if (initial !== undefined) stored.set("endpoint", initial);
  return {
    stored,
    dispose: () => owner.dispose(),
    host: {
      cleanup: (cleanup) => owner.cleanup(cleanup),
      config,
      storage: {
        get: (key: string) => stored.get(key),
        set: (key: string, value: unknown) => {
          stored.set(key, value);
        },
        remove: (key) => {
          stored.delete(key);
        },
        keys: () => [...stored.keys()],
        clear: () => stored.clear(),
      },
    },
  };
}

let runtimeConnection: ReturnType<typeof createRuntimeConnection>;
let endpoint: RuntimeEndpoint;
let configuration: RuntimeEndpointConfiguration;

beforeEach(async () => {
  endpointHost = createHost({ name: "runtime-endpoint-test" });
  endpointHost.install(
    definePlugin({
      name: "test.endpoint-owner",
      setup(ctx) {
        endpointLifetime = ctx.lifetime("endpoint-configurations");
      },
    }),
  );
  await endpointHost.start();
  vi.spyOn(globalThis, "fetch").mockRejectedValue(new Error("Runtime test discovery unavailable"));
  runtimeConnection = createRuntimeConnection(
    {
      ...createBrowserHost(),
      bootstrap: async () => ({
        runtime: { endpoint: DEFAULT_RUNTIME_ENDPOINT },
        localFilesystemEndpoint: null,
      }),
    },
    runtimeRequestMeta,
    createRuntimeMutationJournalStorage(createStorage("test.runtime")),
  );
  await runtimeConnection.initialize();
  config = new Configuration();
});

afterEach(async () => {
  await endpointHost.stop();
  await runtimeConnection.dispose();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

function installConnection(
  initial?: unknown,
  replaceConnection: (commit: () => void) => void = (commit) => commit(),
) {
  const connection = connectionHost(initial);
  configuration = bindRuntimeEndpointConfiguration(connection.host, replaceConnection, () => ({
    endpoint: DEFAULT_RUNTIME_ENDPOINT,
  }));
  endpoint = createRuntimeEndpoint(configuration);
  return connection;
}

function bindConnection(initial?: unknown) {
  const connection = connectionHost(initial);
  const binding = runtimeConnection.bind(connection.host);
  connection.host.cleanup(binding.dispose);
  endpoint = runtimeConnection.access.endpoint;
  return { ...connection, binding };
}

async function bindDesktop(read: () => Promise<unknown>) {
  await runtimeConnection.dispose();
  runtimeConnection = createRuntimeConnection(
    createDesktopHostClient({
      call: async (method) => {
        expect(method).toBe(DesktopMethods.Bootstrap.name);
        return read();
      },
      on: async () => () => {},
    }),
    runtimeRequestMeta,
    createRuntimeMutationJournalStorage(createStorage("test.runtime")),
  );
  await runtimeConnection.initialize();
  const bound = bindConnection();
  await runtimeConnection.access.status.refresh();
  return bound;
}

it("refreshes native credentials through the bound connection recovery path", async () => {
  vi.useFakeTimers();
  let localToken: string | undefined;
  await bindDesktop(async () => ({
    localRuntime: { endpoint: DEFAULT_RUNTIME_ENDPOINT, localToken },
  }));
  expect(endpoint.hasRuntimeAccessToken()).toBe(false);
  const first = runtimeConnection.client();
  const close = vi.spyOn(first, "close");
  localToken = "late-native-token";
  await vi.advanceTimersByTimeAsync(RUNTIME_SERVICE_RETRY_BASE_MS);
  expect(endpoint.hasRuntimeAccessToken()).toBe(true);
  expect(runtimeConnection.client()).not.toBe(first);
  expect(close).toHaveBeenCalledOnce();
  const request = vi.mocked(globalThis.fetch);
  expect(
    new Headers(
      request.mock.calls.findLast(([, init]) => init?.method === "POST")?.[1]?.headers,
    ).get("Authorization"),
  ).toBe("Bearer late-native-token");
});

it("retries failed native reads without making remote connections depend on them", async () => {
  let fails = false;
  let localToken: string | undefined;
  const read = vi.fn(async () => {
    if (fails) throw new Error("native credential read failed");
    return { localRuntime: { endpoint: DEFAULT_RUNTIME_ENDPOINT, localToken } };
  });
  await bindDesktop(read);
  fails = true;
  await runtimeConnection.access.status.refresh();
  expect(runtimeConnection.access.status.snapshot().failure).toMatchObject({
    detail: "desktop host bootstrap failed: native credential read failed",
  });
  fails = false;
  localToken = "recovered-native-token";
  await runtimeConnection.access.status.refresh();
  expect(endpoint.hasRuntimeAccessToken()).toBe(true);
  fails = true;
  const reads = read.mock.calls.length;
  endpoint.applyRuntimeEndpoint("https://remote.example", "remote-token");
  await runtimeConnection.access.status.refresh();
  expect(read).toHaveBeenCalledTimes(reads);
  expect(runtimeConnection.access.status.snapshot().failure).toMatchObject({
    detail: expect.stringContaining("Runtime test discovery unavailable"),
  });
});

it("reset selects the host observation even when a configured token currently matches it", async () => {
  let localToken = "native-before";
  await bindDesktop(async () => ({
    localRuntime: { endpoint: DEFAULT_RUNTIME_ENDPOINT, localToken },
  }));
  endpoint.applyRuntimeEndpoint(DEFAULT_RUNTIME_ENDPOINT, "configured-token");
  await runtimeConnection.access.status.refresh();
  localToken = "configured-token";
  await runtimeConnection.initialize();
  expect(endpoint.resetRuntimeEndpoint()).toMatchObject({ changed: true });
  await runtimeConnection.access.status.refresh();
  localToken = "native-after";
  await runtimeConnection.access.status.refresh();
  const request = vi.mocked(globalThis.fetch);
  expect(
    new Headers(
      request.mock.calls.findLast(([, init]) => init?.method === "POST")?.[1]?.headers,
    ).get("Authorization"),
  ).toBe("Bearer native-after");
});

it("fences a native recovery read when the user replaces its connection", async () => {
  let pending: Promise<unknown> | undefined;
  const entered = Promise.withResolvers<void>();
  await bindDesktop(async () => {
    if (pending) {
      entered.resolve();
      return pending;
    }
    return { localRuntime: { endpoint: DEFAULT_RUNTIME_ENDPOINT } };
  });
  const retired = Promise.withResolvers<unknown>();
  pending = retired.promise;
  const recovering = runtimeConnection.access.status.refresh();
  await entered.promise;
  endpoint.applyRuntimeEndpoint("https://replacement.example", "replacement-token");
  await runtimeConnection.access.status.refresh();
  retired.resolve({
    localRuntime: { endpoint: "https://retired.example", localToken: "retired-token" },
  });
  await recovering;
  await Promise.resolve();
  expect(endpoint.currentRuntimeEndpoint()).toBe("https://replacement.example");
  expect(runtimeConnection.bootstrap().runtime).toEqual({ endpoint: DEFAULT_RUNTIME_ENDPOINT });
  const request = vi.mocked(globalThis.fetch);
  expect(
    new Headers(
      request.mock.calls.findLast(([, init]) => init?.method === "POST")?.[1]?.headers,
    ).get("Authorization"),
  ).toBe("Bearer replacement-token");
});

it("keeps a configured remote credential separate from native refresh and resets to the live host", async () => {
  let localToken = "native-before";
  await bindDesktop(async () => ({
    localRuntime: { endpoint: DEFAULT_RUNTIME_ENDPOINT, localToken },
  }));
  endpoint.applyRuntimeEndpoint("https://remote.example", "remote-token");
  await runtimeConnection.access.status.refresh();
  localToken = "native-after";
  await runtimeConnection.access.status.refresh();
  expect(endpoint.currentRuntimeEndpoint()).toBe("https://remote.example");
  const request = vi.mocked(globalThis.fetch);
  expect(
    new Headers(
      request.mock.calls.findLast(([, init]) => init?.method === "POST")?.[1]?.headers,
    ).get("Authorization"),
  ).toBe("Bearer remote-token");
  endpoint.resetRuntimeEndpoint();
  await runtimeConnection.access.status.refresh();
  expect(
    new Headers(
      request.mock.calls.findLast(([, init]) => init?.method === "POST")?.[1]?.headers,
    ).get("Authorization"),
  ).toBe("Bearer native-after");
});

describe("runtime endpoint", () => {
  it("requires the environment bootstrap before exposing a Runtime endpoint", async () => {
    const owner = createRuntimeConnection(
      createBrowserHost(),
      runtimeRequestMeta,
      createRuntimeMutationJournalStorage(createStorage("test.uninitialized")),
    );
    expect(() => owner.access.endpoint.currentRuntimeEndpoint()).toThrow(
      "client host has not completed bootstrap",
    );
    expect(() => owner.access.endpoint.applyRuntimeEndpoint("http://127.0.0.1:27171")).toThrow(
      "client host has not completed bootstrap",
    );
    await owner.dispose();
  });

  it("restores the persisted endpoint before Runtime discovery starts", () => {
    installConnection("http://127.0.0.1:27171");

    expect(endpoint.currentRuntimeEndpoint()).toBe("http://127.0.0.1:27171");
  });

  it("ignores a persisted endpoint with the wrong runtime type", () => {
    installConnection(27171);

    expect(endpoint.currentRuntimeEndpoint()).toBe(DEFAULT_RUNTIME_ENDPOINT);
  });

  it("validates, normalizes, and publishes a changed endpoint", () => {
    installConnection();

    const result = endpoint.applyRuntimeEndpoint("  http://127.0.0.1:27171  ");

    expect(result).toEqual({
      kind: "applied",
      endpoint: "http://127.0.0.1:27171",
      changed: true,
    });
    expect(endpoint.currentRuntimeEndpoint()).toBe("http://127.0.0.1:27171");
  });

  it("commits a changed endpoint inside the Runtime connection replacement", () => {
    const order: string[] = [];
    installConnection(undefined, (commit) => {
      order.push(`before:${endpoint.currentRuntimeEndpoint()}`);
      commit();
      order.push(`after:${endpoint.currentRuntimeEndpoint()}`);
    });

    endpoint.applyRuntimeEndpoint("http://127.0.0.1:27171");

    expect(order).toEqual([`before:${DEFAULT_RUNTIME_ENDPOINT}`, "after:http://127.0.0.1:27171"]);
  });

  it("rejects invalid input without changing the active endpoint", () => {
    installConnection();

    const result = endpoint.applyRuntimeEndpoint("file:///tmp/runtime.sock");

    expect(result).toEqual({
      kind: "rejected",
      input: "file:///tmp/runtime.sock",
      reason: "unsupported_scheme",
    });
    expect(endpoint.currentRuntimeEndpoint()).toBe(DEFAULT_RUNTIME_ENDPOINT);
  });

  it("distinguishes malformed URLs from unsupported schemes", () => {
    installConnection();

    expect(endpoint.applyRuntimeEndpoint("not a URL")).toEqual({
      kind: "rejected",
      input: "not a URL",
      reason: "invalid_url",
    });
  });

  it("persists published changes through the Runtime-owned adapter", () => {
    const { stored } = installConnection();

    endpoint.applyRuntimeEndpoint("http://127.0.0.1:27171");

    expect(stored.get("endpoint")).toBe("http://127.0.0.1:27171");
  });

  it("retires the storage mirror with the Runtime endpoint owner", async () => {
    const connection = connectionHost();
    configuration = bindRuntimeEndpointConfiguration(
      connection.host,
      (commit) => commit(),
      () => ({
        endpoint: DEFAULT_RUNTIME_ENDPOINT,
      }),
    );
    endpoint = createRuntimeEndpoint(configuration);

    endpoint.applyRuntimeEndpoint("http://127.0.0.1:27171");
    await connection.dispose();
    config.set("runtime.endpoint", "http://127.0.0.1:28181");

    expect(connection.stored.get("endpoint")).toBe("http://127.0.0.1:27171");
  });

  it("cannot return an endpoint owner when installing its configuration subscription fails", async () => {
    const connection = connectionHost();
    connection.host.config.onChange = () => {
      throw new Error("configuration subscription failed");
    };
    let configured: RuntimeEndpointConfiguration | undefined;
    expect(() => {
      configured = bindRuntimeEndpointConfiguration(
        connection.host,
        (commit) => commit(),
        () => ({
          endpoint: DEFAULT_RUNTIME_ENDPOINT,
        }),
      );
    }).toThrow("configuration subscription failed");
    await connection.dispose();
    expect(configured).toBeUndefined();
  });

  it("clears credentials even when its configuration subscription cleanup fails", async () => {
    const connection = connectionHost();
    connection.host.config.onChange = () => ({
      dispose() {
        throw new Error("configuration unsubscribe failed");
      },
    });
    configuration = bindRuntimeEndpointConfiguration(
      connection.host,
      (commit) => commit(),
      () => ({
        endpoint: DEFAULT_RUNTIME_ENDPOINT,
      }),
    );
    endpoint = createRuntimeEndpoint(configuration);

    endpoint.applyRuntimeEndpoint(DEFAULT_RUNTIME_ENDPOINT, "retired-secret");
    expect(configuration.read().localToken).toBe("retired-secret");
    await expect(connection.dispose()).rejects.toThrow("configuration unsubscribe failed");
    expect(configuration.read().localToken).toBeUndefined();
  });

  it("resets to the default endpoint with honest change metadata", () => {
    installConnection();
    expect(endpoint.defaultRuntimeEndpoint()).toBe(DEFAULT_RUNTIME_ENDPOINT);
    endpoint.applyRuntimeEndpoint("http://127.0.0.1:27171");

    expect(endpoint.resetRuntimeEndpoint()).toEqual({
      kind: "applied",
      endpoint: DEFAULT_RUNTIME_ENDPOINT,
      changed: true,
    });
  });

  it("rebuilds the shared Runtime client after an endpoint change", () => {
    bindConnection();
    const first = runtimeConnection.client();

    endpoint.applyRuntimeEndpoint("http://127.0.0.1:27171");
    const second = runtimeConnection.client();

    expect(second).not.toBe(first);
    expect(runtimeConnection.client()).toBe(second);
  });
});

describe("Runtime target credentials", () => {
  it("replaces the connection on token changes and never persists the token", () => {
    const { stored, binding } = bindConnection(DEFAULT_RUNTIME_ENDPOINT);
    const publication = vi.spyOn(config, "set");
    const replacements = vi.spyOn(binding.owner, "replaceEndpoint");
    const first = runtimeConnection.client();
    expect(endpoint.applyRuntimeEndpoint(DEFAULT_RUNTIME_ENDPOINT, "window-secret")).toMatchObject({
      kind: "applied",
      changed: true,
    });
    expect(replacements).toHaveBeenCalledOnce();
    expect(runtimeConnection.client()).not.toBe(first);
    expect([...stored.values()]).toEqual([DEFAULT_RUNTIME_ENDPOINT]);
    expect(publication.mock.calls.map(([, value]) => value)).toEqual([DEFAULT_RUNTIME_ENDPOINT]);
    expect(endpoint.applyRuntimeEndpoint(DEFAULT_RUNTIME_ENDPOINT, "window-secret")).toMatchObject({
      kind: "applied",
      changed: false,
    });
    expect(replacements).toHaveBeenCalledOnce();
  });

  it("clears credentials when changing addresses and starts fresh after reinstall", async () => {
    const connection = connectionHost();
    configuration = bindRuntimeEndpointConfiguration(
      connection.host,
      (commit) => commit(),
      () => ({
        endpoint: DEFAULT_RUNTIME_ENDPOINT,
      }),
    );
    endpoint = createRuntimeEndpoint(configuration);
    endpoint.applyRuntimeEndpoint(DEFAULT_RUNTIME_ENDPOINT, "window-secret");
    endpoint.applyRuntimeEndpoint("https://another.example");
    expect(configuration.read()).toEqual({
      endpoint: "https://another.example",
      localToken: undefined,
    });
    await connection.dispose();
    installConnection("https://another.example");
    expect(configuration.read()).toEqual({
      endpoint: "https://another.example",
      localToken: undefined,
    });
  });

  it.each([
    "https://user:secret@flame.example",
    "https://flame.example?token=secret",
    "https://flame.example#secret",
  ])("rejects credential-bearing endpoint %s", (input) => {
    installConnection();
    expect(endpoint.applyRuntimeEndpoint(input).kind).toBe("rejected");
    expect(endpoint.currentRuntimeEndpoint()).toBe(DEFAULT_RUNTIME_ENDPOINT);
  });
});

it("rejects malformed credentials before replacing the connection", () => {
  let replacements = 0;
  installConnection(undefined, (commit) => {
    replacements++;
    commit();
  });
  expect(endpoint.applyRuntimeEndpoint(DEFAULT_RUNTIME_ENDPOINT, "bad\r\ntoken")).toMatchObject({
    kind: "rejected",
    reason: "invalid_token",
  });
  expect(configuration.read()?.localToken).toBeUndefined();
  expect(replacements).toBe(0);
});

it("binds the local token and filesystem capability only to the bootstrapped Runtime", async () => {
  const localEndpoint = "http://127.0.0.1:17172";
  await runtimeConnection.dispose();
  runtimeConnection = createRuntimeConnection(
    {
      ...createBrowserHost(),
      kind: "desktop",
      bootstrap: async () => ({
        runtime: { endpoint: localEndpoint, localToken: "native-token" },
        localFilesystemEndpoint: localEndpoint,
      }),
    },
    runtimeRequestMeta,
    createRuntimeMutationJournalStorage(createStorage("test.runtime")),
  );
  await runtimeConnection.initialize();
  const connection = connectionHost();
  const binding = runtimeConnection.bind(connection.host);
  connection.host.cleanup(binding.dispose);
  endpoint = runtimeConnection.access.endpoint;
  const request = vi
    .spyOn(globalThis, "fetch")
    .mockRejectedValue(new Error("captured request"))
    .mockClear();
  expect(runtimeConnection.localWorkspaceAvailable()).toBe(true);
  await expect(runtimeConnection.client().runtime.discover()).rejects.toThrow("captured request");
  expect(
    new Headers(
      request.mock.calls.findLast(([, init]) => init?.method === "POST")?.[1]?.headers,
    ).get("Authorization"),
  ).toBe("Bearer native-token");
  endpoint.applyRuntimeEndpoint("http://127.0.0.1:17171");
  expect(runtimeConnection.localWorkspaceAvailable()).toBe(false);
  await expect(runtimeConnection.client().runtime.discover()).rejects.toThrow("captured request");
  expect(
    new Headers(
      request.mock.calls.findLast(([, init]) => init?.method === "POST")?.[1]?.headers,
    ).get("Authorization"),
  ).toBeNull();
  endpoint.applyRuntimeEndpoint("https://remote.example");
  expect(runtimeConnection.localWorkspaceAvailable()).toBe(false);
  await expect(runtimeConnection.client().runtime.discover()).rejects.toThrow("captured request");
  expect(
    new Headers(
      request.mock.calls.findLast(([, init]) => init?.method === "POST")?.[1]?.headers,
    ).get("Authorization"),
  ).toBeNull();
  endpoint.resetRuntimeEndpoint();
  expect(runtimeConnection.bootstrap().runtime).toEqual({
    endpoint: localEndpoint,
    localToken: "native-token",
  });
  expect(runtimeConnection.localWorkspaceAvailable()).toBe(true);
  await expect(runtimeConnection.client().runtime.discover()).rejects.toThrow("captured request");
  expect(
    new Headers(
      request.mock.calls.findLast(([, init]) => init?.method === "POST")?.[1]?.headers,
    ).get("Authorization"),
  ).toBe("Bearer native-token");
});

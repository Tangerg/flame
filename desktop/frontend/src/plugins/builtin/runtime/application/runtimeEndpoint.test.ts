import { createBrowserHost } from "@/platform/browserHost";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getContainer, initializeClientHost, resetContainer, setContainer } from "@/main/container";
import { getConfig, hasConfig, setConfig, useConfigStore } from "@/plugins/sdk/config";
import type { ConfigService, KeyValueStore } from "@/plugins/sdk";
import {
  applyRuntimeEndpoint,
  currentRuntimeEndpoint,
  configuredRuntimeTarget,
  defaultRuntimeEndpoint,
  resetRuntimeEndpoint,
} from "./runtimeEndpoint";
import { installRuntimeEndpointConfiguration } from "../adapters/runtimeEndpointConfiguration";

const DEFAULT_RUNTIME_ENDPOINT = "https://flame.example";

const cleanups: Array<() => void> = [];

function connectionHost(initial?: unknown): {
  host: { config: ConfigService; storage: KeyValueStore };
  stored: Map<string, unknown>;
} {
  const stored = new Map<string, unknown>();
  if (initial !== undefined) stored.set("endpoint", initial);
  return {
    stored,
    host: {
      config: {
        get: getConfig,
        set: setConfig,
        has: hasConfig,
        onChange: (key, fn) => useConfigStore.getState().subscribe(key, fn),
      },
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

beforeEach(() => {
  useConfigStore.setState({ values: new Map(), subscribers: new Map() });
});

afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) cleanup();
  await resetContainer();
  vi.restoreAllMocks();
});

function installConnection(
  initial?: unknown,
  replaceConnection: (commit: () => void) => void = (commit) => commit(),
) {
  const connection = connectionHost(initial);
  cleanups.push(
    installRuntimeEndpointConfiguration(connection.host, replaceConnection, {
      endpoint: DEFAULT_RUNTIME_ENDPOINT,
    }),
  );
  return connection;
}

describe("runtime endpoint", () => {
  it("requires the environment bootstrap before exposing a Runtime endpoint", () => {
    expect(() => currentRuntimeEndpoint()).toThrow(
      "Runtime endpoint configuration is not installed",
    );
    expect(() => applyRuntimeEndpoint("http://127.0.0.1:27171")).toThrow(
      "Runtime endpoint configuration is not installed",
    );
  });

  it("restores the persisted endpoint before Runtime discovery starts", () => {
    installConnection("http://127.0.0.1:27171");

    expect(currentRuntimeEndpoint()).toBe("http://127.0.0.1:27171");
  });

  it("ignores a persisted endpoint with the wrong runtime type", () => {
    installConnection(27171);

    expect(currentRuntimeEndpoint()).toBe(DEFAULT_RUNTIME_ENDPOINT);
  });

  it("validates, normalizes, and publishes a changed endpoint", () => {
    installConnection();

    const result = applyRuntimeEndpoint("  http://127.0.0.1:27171  ");

    expect(result).toEqual({
      kind: "applied",
      endpoint: "http://127.0.0.1:27171",
      changed: true,
    });
    expect(currentRuntimeEndpoint()).toBe("http://127.0.0.1:27171");
  });

  it("commits a changed endpoint inside the Runtime connection replacement", () => {
    const order: string[] = [];
    installConnection(undefined, (commit) => {
      order.push(`before:${currentRuntimeEndpoint()}`);
      commit();
      order.push(`after:${currentRuntimeEndpoint()}`);
    });

    applyRuntimeEndpoint("http://127.0.0.1:27171");

    expect(order).toEqual([`before:${DEFAULT_RUNTIME_ENDPOINT}`, "after:http://127.0.0.1:27171"]);
  });

  it("rejects invalid input without changing the active endpoint", () => {
    installConnection();

    const result = applyRuntimeEndpoint("file:///tmp/runtime.sock");

    expect(result).toEqual({
      kind: "rejected",
      input: "file:///tmp/runtime.sock",
      reason: "unsupported_scheme",
    });
    expect(currentRuntimeEndpoint()).toBe(DEFAULT_RUNTIME_ENDPOINT);
  });

  it("distinguishes malformed URLs from unsupported schemes", () => {
    installConnection();

    expect(applyRuntimeEndpoint("not a URL")).toEqual({
      kind: "rejected",
      input: "not a URL",
      reason: "invalid_url",
    });
  });

  it("persists published changes through the Runtime-owned adapter", () => {
    const { stored } = installConnection();

    applyRuntimeEndpoint("http://127.0.0.1:27171");

    expect(stored.get("endpoint")).toBe("http://127.0.0.1:27171");
  });

  it("retires the storage mirror with the Runtime endpoint owner", () => {
    const connection = connectionHost();
    const dispose = installRuntimeEndpointConfiguration(connection.host, (commit) => commit(), {
      endpoint: DEFAULT_RUNTIME_ENDPOINT,
    });

    applyRuntimeEndpoint("http://127.0.0.1:27171");
    dispose();
    setConfig("runtime.endpoint", "http://127.0.0.1:28181");

    expect(connection.stored.get("endpoint")).toBe("http://127.0.0.1:27171");
  });

  it("resets to the default endpoint with honest change metadata", () => {
    installConnection();
    expect(defaultRuntimeEndpoint()).toBe(DEFAULT_RUNTIME_ENDPOINT);
    applyRuntimeEndpoint("http://127.0.0.1:27171");

    expect(resetRuntimeEndpoint()).toEqual({
      kind: "applied",
      endpoint: DEFAULT_RUNTIME_ENDPOINT,
      changed: true,
    });
  });

  it("rebuilds the shared Runtime client after an endpoint change", () => {
    installConnection();
    const first = getContainer().client();

    applyRuntimeEndpoint("http://127.0.0.1:27171");
    const second = getContainer().client();

    expect(second).not.toBe(first);
    expect(getContainer().client()).toBe(second);
  });
});

describe("Runtime target credentials", () => {
  it("replaces the connection on token changes and never persists the token", () => {
    let replacements = 0;
    const { stored } = installConnection(DEFAULT_RUNTIME_ENDPOINT, (commit) => {
      replacements++;
      commit();
    });
    const first = getContainer().client();
    expect(applyRuntimeEndpoint(DEFAULT_RUNTIME_ENDPOINT, "window-secret")).toMatchObject({
      kind: "applied",
      changed: true,
    });
    expect(replacements).toBe(1);
    expect(getContainer().client()).not.toBe(first);
    expect([...stored.values()]).toEqual([DEFAULT_RUNTIME_ENDPOINT]);
    expect([...useConfigStore.getState().values.values()]).not.toContain("window-secret");
    expect(applyRuntimeEndpoint(DEFAULT_RUNTIME_ENDPOINT, "window-secret")).toMatchObject({
      kind: "applied",
      changed: false,
    });
    expect(replacements).toBe(1);
  });

  it("clears credentials when changing addresses and starts fresh after reinstall", () => {
    const connection = connectionHost();
    const dispose = installRuntimeEndpointConfiguration(connection.host, (commit) => commit(), {
      endpoint: DEFAULT_RUNTIME_ENDPOINT,
    });
    applyRuntimeEndpoint(DEFAULT_RUNTIME_ENDPOINT, "window-secret");
    applyRuntimeEndpoint("https://another.example");
    expect(configuredRuntimeTarget()).toEqual({
      endpoint: "https://another.example",
      localToken: undefined,
    });
    dispose();
    cleanups.push(
      installRuntimeEndpointConfiguration(connection.host, (commit) => commit(), {
        endpoint: DEFAULT_RUNTIME_ENDPOINT,
      }),
    );
    expect(configuredRuntimeTarget()).toEqual({
      endpoint: "https://another.example",
      localToken: undefined,
    });
  });

  it.each([
    "https://user:secret@flame.example",
    "https://flame.example?token=secret",
    "https://flame.example#secret",
  ])("rejects credential-bearing endpoint %s", (endpoint) => {
    installConnection();
    expect(applyRuntimeEndpoint(endpoint).kind).toBe("rejected");
    expect(currentRuntimeEndpoint()).toBe(DEFAULT_RUNTIME_ENDPOINT);
  });
});

it("rejects malformed credentials before replacing the connection", () => {
  let replacements = 0;
  installConnection(undefined, (commit) => {
    replacements++;
    commit();
  });
  expect(applyRuntimeEndpoint(DEFAULT_RUNTIME_ENDPOINT, "bad\r\ntoken")).toMatchObject({
    kind: "rejected",
    reason: "invalid_token",
  });
  expect(configuredRuntimeTarget()?.localToken).toBeUndefined();
  expect(replacements).toBe(0);
});

it("binds the local token and filesystem capability only to the bootstrapped Runtime", async () => {
  const endpoint = "http://127.0.0.1:17171";
  setContainer({
    host: {
      ...createBrowserHost(),
      kind: "desktop",
      bootstrap: async () => ({
        runtime: { endpoint, localToken: "native-token" },
        localFilesystemEndpoint: endpoint,
      }),
    },
  });
  await initializeClientHost();
  const connection = connectionHost();
  cleanups.push(installRuntimeEndpointConfiguration(connection.host, (commit) => commit()));
  const request = vi.spyOn(globalThis, "fetch").mockRejectedValue(new Error("captured request"));
  expect(getContainer().localWorkspaceAvailable()).toBe(true);
  await expect(getContainer().client().runtime.discover()).rejects.toThrow("captured request");
  expect(new Headers(request.mock.calls[0]?.[1]?.headers).get("Authorization")).toBe(
    "Bearer native-token",
  );
  applyRuntimeEndpoint("https://remote.example");
  expect(getContainer().localWorkspaceAvailable()).toBe(false);
  await expect(getContainer().client().runtime.discover()).rejects.toThrow("captured request");
  expect(new Headers(request.mock.calls[1]?.[1]?.headers).get("Authorization")).toBeNull();
});

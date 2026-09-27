import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ClientBootstrap, ClientHost } from "@/platform/host";
import { createBrowserHost } from "@/platform/browserHost";
import { createRuntimeConnection } from "./runtimeConnection";

const LOCAL_ENDPOINT = "http://127.0.0.1:17171";

function localBootstrap(localToken: string): ClientBootstrap {
  return {
    runtime: { endpoint: LOCAL_ENDPOINT, localToken },
    localFilesystemEndpoint: LOCAL_ENDPOINT,
  };
}

function desktop(bootstrap: ClientHost["bootstrap"]): ClientHost {
  return { ...createBrowserHost(), kind: "desktop", bootstrap };
}

let connection: ReturnType<typeof createRuntimeConnection>;
beforeEach(async () => {
  connection = createRuntimeConnection(createBrowserHost());
  await connection.initialize();
});
afterEach(async () => {
  await connection.dispose();
  vi.restoreAllMocks();
});

describe("Runtime connection ownership", () => {
  it("uses the injected host bootstrap and scopes local filesystem access to it", () => {
    expect(connection.bootstrap().runtime.endpoint).toBe(window.location.origin);
    expect(connection.localWorkspaceAvailable()).toBe(false);
  });

  it("requires each owner to finish its own bootstrap", async () => {
    const successor = createRuntimeConnection(createBrowserHost());
    expect(() => successor.client()).toThrow("client host has not completed bootstrap");
    await successor.initialize();
    expect(successor.client()).not.toBe(connection.client());
    await successor.dispose();
  });

  it("caches the Runtime client and sidecar within their endpoint lifetime", () => {
    expect(connection.client()).toBe(connection.client());
    expect(connection.sidecar()).toBe(connection.sidecar());
  });

  it("joins its connection once and cannot resurrect resources after final close", async () => {
    const close = vi.spyOn(connection.client(), "close");
    connection.sidecar();
    const closing = connection.dispose();
    expect(connection.dispose()).toBe(closing);
    expect(() => connection.client()).toThrow("Runtime connection is closed");
    expect(() => connection.sidecar()).toThrow("Runtime connection is closed");
    await closing;
    expect(close).toHaveBeenCalledOnce();
    await expect(connection.initialize()).rejects.toThrow("Runtime connection is closed");
  });

  it("never closes another renderer's connection", async () => {
    const successor = createRuntimeConnection(createBrowserHost());
    await successor.initialize();
    const close = vi.spyOn(successor.client(), "close");
    await connection.dispose();
    expect(close).not.toHaveBeenCalled();
    expect(successor.client()).toBe(successor.client());
    await successor.dispose();
  });

  it("retires the previous client when the same host refreshes its credential", async () => {
    let token = "token-a";
    await connection.dispose();
    connection = createRuntimeConnection(desktop(async () => localBootstrap(token)));
    await connection.initialize();
    const first = connection.client();
    const close = vi.spyOn(first, "close");
    expect(connection.localWorkspaceAvailable()).toBe(true);
    token = "token-b";
    await connection.initialize();
    const second = connection.client();
    expect(second).not.toBe(first);
    expect(close).toHaveBeenCalledOnce();
    expect(connection.client()).toBe(second);
  });

  it("fences an earlier bootstrap completion within one owner", async () => {
    const retired = Promise.withResolvers<ClientBootstrap>();
    const bootstrap = vi
      .fn()
      .mockReturnValueOnce(retired.promise)
      .mockResolvedValue(localBootstrap("successor-token"));
    await connection.dispose();
    connection = createRuntimeConnection(desktop(bootstrap));
    const pending = connection.initialize();
    await connection.initialize();
    retired.resolve(localBootstrap("retired-token"));
    await pending;
    expect(connection.bootstrap().runtime.localToken).toBe("successor-token");
  });

  it("fences late bootstrap completion after that owner is disposed", async () => {
    const retired = Promise.withResolvers<ClientBootstrap>();
    const predecessor = createRuntimeConnection(desktop(() => retired.promise));
    const pending = predecessor.initialize();
    await predecessor.dispose();
    retired.resolve(localBootstrap("retired-token"));
    await pending;
    expect(() => predecessor.bootstrap()).toThrow("Runtime connection is closed");
    expect(connection.bootstrap().runtime.localToken).toBeUndefined();
  });
});

it("joins retired clients and preserves their cleanup failures through final disposal", async () => {
  let token = "old";
  const owner = createRuntimeConnection(desktop(async () => localBootstrap(token)));
  await owner.initialize();
  const failure = new Error("transport cleanup failed");
  const retiredClose = vi.spyOn(owner.client(), "close").mockRejectedValue(failure);
  token = "new";
  await owner.initialize();
  const currentClose = vi.spyOn(owner.client(), "close");
  await Promise.resolve();
  const closing = owner.dispose();
  await expect(closing).rejects.toMatchObject({
    message: "Runtime connection cleanup failed",
    errors: [failure],
  });
  expect(owner.dispose()).toBe(closing);
  expect(retiredClose).toHaveBeenCalledOnce();
  expect(currentClose).toHaveBeenCalledOnce();
});

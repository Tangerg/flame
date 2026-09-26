import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { FlameClient } from "@flame/runtime-contract/client";
import type { ClientBootstrap, ClientHost } from "@/platform/host";
import { createBrowserHost } from "@/platform/browserHost";
import {
  disposeContainer,
  getContainer,
  initializeClientHost,
  resetContainer,
  setContainer,
} from "./container";

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

describe("main/container", () => {
  beforeEach(initializeClientHost);
  afterEach(resetContainer);
  afterEach(() => vi.restoreAllMocks());

  it("exposes Runtime entry points and its actual host", () => {
    const container = getContainer();
    expect(typeof container.client).toBe("function");
    expect(typeof container.sidecar).toBe("function");
    expect(container.host.kind).toBe("web");
    expect(container.bootstrap().runtime.endpoint).toBe(window.location.origin);
    expect(container.localWorkspaceAvailable()).toBe(false);
  });

  it("setContainer swaps only the injected slot", () => {
    const fake = {} as FlameClient;
    const before = getContainer().host;
    setContainer({ client: () => fake });
    expect(getContainer().client()).toBe(fake);
    expect(getContainer().host).toBe(before);
  });

  it("requires a fresh bootstrap after reset and joins the former client's teardown", async () => {
    const first = getContainer().client();
    expect(getContainer().client()).toBe(first);
    const close = vi.spyOn(first, "close");
    await resetContainer();
    expect(close).toHaveBeenCalledOnce();
    expect(() => getContainer().client()).toThrow("client host has not completed bootstrap");
    await initializeClientHost();
    expect(getContainer().client()).not.toBe(first);
  });

  it("joins its connection once and does not resurrect resources after final close", async () => {
    const owner = getContainer();
    const close = vi.spyOn(owner.client(), "close");
    owner.sidecar();
    const closing = disposeContainer();
    expect(disposeContainer()).toBe(closing);
    expect(() => owner.client()).toThrow("Client container is closed");
    expect(() => owner.sidecar()).toThrow("Client container is closed");
    await closing;
    expect(close).toHaveBeenCalledOnce();
    expect(() => owner.client()).toThrow("Client container is closed");
  });

  it("does not close a client injected by an external owner", async () => {
    const close = vi.fn(async () => {});
    setContainer({ client: () => ({ close }) as unknown as FlameClient });
    await disposeContainer();
    expect(close).not.toHaveBeenCalled();
  });

  it("retires the previous connection when the host changes its token", async () => {
    setContainer({ host: desktop(async () => localBootstrap("token-a")) });
    await initializeClientHost();
    const first = getContainer().client();
    const close = vi.spyOn(first, "close");
    expect(getContainer().localWorkspaceAvailable()).toBe(true);
    setContainer({ host: desktop(async () => localBootstrap("token-b")) });
    await initializeClientHost();
    const second = getContainer().client();
    expect(second).not.toBe(first);
    expect(close).toHaveBeenCalledOnce();
    expect(getContainer().client()).toBe(second);
  });

  it.each(["container", "host"])(
    "fences late bootstrap results after replacing the %s",
    async (replacement) => {
      const retired = Promise.withResolvers<ClientBootstrap>();
      setContainer({ host: desktop(() => retired.promise) });
      const pending = initializeClientHost();
      if (replacement === "container") await resetContainer();
      setContainer({ host: desktop(async () => localBootstrap("successor-token")) });
      await initializeClientHost();
      retired.resolve(localBootstrap("retired-token"));
      await pending;
      const request = vi
        .spyOn(globalThis, "fetch")
        .mockRejectedValueOnce(new Error("request captured"));
      await expect(getContainer().client().runtime.discover()).rejects.toThrow("request captured");
      expect(new Headers(request.mock.calls[0]?.[1]?.headers).get("Authorization")).toBe(
        "Bearer successor-token",
      );
    },
  );

  it("caches sidecars for the actual bootstrapped endpoint", async () => {
    const first = getContainer().sidecar();
    expect(getContainer().sidecar()).toBe(first);
    await resetContainer();
    await initializeClientHost();
    expect(getContainer().sidecar()).not.toBe(first);
  });
});

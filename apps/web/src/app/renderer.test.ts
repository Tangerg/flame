import { describe, expect, it, vi } from "vitest";
import { ClientRenderer, type ClientRendererDependencies } from "@/app/renderer";
function rendererDependencies(overrides: Partial<ClientRendererDependencies> = {}) {
  return {
    initializeClientHost: vi.fn(async () => undefined),
    prepareWindowChrome: vi.fn(async () => undefined),
    watchWindowChrome: vi.fn(() => vi.fn()),
    mount: vi.fn(() => ({ unmount: vi.fn() })),
    mountStartupFailure: vi.fn(() => ({ unmount: vi.fn() })),
    closeConnection: vi.fn(async () => undefined),
    reportFailure: vi.fn(),
    ...overrides,
  };
}

describe("ClientRenderer", () => {
  it("cannot mount after final close wins an in-flight bootstrap", async () => {
    const bootstrap = Promise.withResolvers<void>();
    const deps = rendererDependencies({
      initializeClientHost: vi.fn(() => bootstrap.promise),
    });
    const renderer = new ClientRenderer(deps);

    const startup = renderer.start();
    const closing = renderer.dispose();
    bootstrap.resolve();

    await Promise.all([startup, closing]);
    expect(deps.prepareWindowChrome).not.toHaveBeenCalled();
    expect(deps.watchWindowChrome).not.toHaveBeenCalled();
    expect(deps.mount).not.toHaveBeenCalled();
    expect(deps.closeConnection).toHaveBeenCalledOnce();
  });

  it("mounts once however many times start is called", async () => {
    const deps = rendererDependencies();
    const renderer = new ClientRenderer(deps);

    await Promise.all([renderer.start(), renderer.start()]);
    await renderer.start();

    expect(deps.mount).toHaveBeenCalledOnce();
    expect(deps.watchWindowChrome).toHaveBeenCalledOnce();
    await renderer.dispose();
  });

  it("cannot mount a failure after final close wins a rejected bootstrap", async () => {
    const bootstrap = Promise.withResolvers<void>();
    const deps = rendererDependencies({
      initializeClientHost: vi.fn(() => bootstrap.promise),
    });
    const renderer = new ClientRenderer(deps);
    const startup = renderer.start().catch((error: unknown) => error);
    await renderer.dispose();
    bootstrap.reject(new Error("retired host"));
    await expect(startup).resolves.toEqual(new Error("retired host"));
    expect(deps.mount).not.toHaveBeenCalled();
    expect(deps.mountStartupFailure).not.toHaveBeenCalled();
    expect(deps.closeConnection).toHaveBeenCalledOnce();
  });

  it("cannot mount a failure after final close wins startup cleanup", async () => {
    const closing = Promise.withResolvers<void>();
    const deps = rendererDependencies({
      initializeClientHost: vi.fn(async () => {
        throw new Error("host unavailable");
      }),
      closeConnection: vi.fn(() => closing.promise),
    });
    const renderer = new ClientRenderer(deps);
    const startup = renderer.start().catch((error: unknown) => error);
    await vi.waitFor(() => expect(deps.closeConnection).toHaveBeenCalledOnce());
    const disposal = renderer.dispose();
    closing.resolve();
    await Promise.all([startup, disposal]);
    await expect(startup).resolves.toEqual(new Error("host unavailable"));
    expect(deps.mountStartupFailure).not.toHaveBeenCalled();
    expect(deps.closeConnection).toHaveBeenCalledOnce();
  });

  it("does not start after the window has already been closed", async () => {
    const deps = rendererDependencies();
    const renderer = new ClientRenderer(deps);

    await renderer.dispose();
    await renderer.start();

    expect(deps.initializeClientHost).not.toHaveBeenCalled();
    expect(deps.mount).not.toHaveBeenCalled();
  });

  it("cannot install window owners after close wins chrome preparation", async () => {
    const chrome = Promise.withResolvers<void>();
    const deps = rendererDependencies({
      prepareWindowChrome: vi.fn(() => chrome.promise),
    });
    const renderer = new ClientRenderer(deps);

    const startup = renderer.start();
    await vi.waitFor(() => expect(deps.prepareWindowChrome).toHaveBeenCalledOnce());
    const closing = renderer.dispose();
    chrome.resolve();

    await Promise.all([startup, closing]);
    expect(deps.watchWindowChrome).not.toHaveBeenCalled();
    expect(deps.mount).not.toHaveBeenCalled();
  });

  it("retires the mounted root and watcher synchronously with one close settlement", async () => {
    const unmount = vi.fn();
    const stopWatching = vi.fn();
    const runtimeClose = Promise.withResolvers<void>();
    const deps = rendererDependencies({
      watchWindowChrome: vi.fn(() => stopWatching),
      mount: vi.fn(() => ({ unmount })),
      closeConnection: vi.fn(() => runtimeClose.promise),
    });
    const renderer = new ClientRenderer(deps);
    await renderer.start();

    const first = renderer.dispose();
    const second = renderer.dispose();

    expect(second).toBe(first);
    expect(unmount).toHaveBeenCalledOnce();
    expect(stopWatching).toHaveBeenCalledOnce();
    expect(deps.closeConnection).toHaveBeenCalledOnce();
    runtimeClose.resolve();
    await first;
  });
});

it("owns the failure root until teardown while closing a failed client's owners immediately", async () => {
  const unmountFailure = vi.fn();
  const deps = rendererDependencies({
    initializeClientHost: vi.fn(async () => {
      throw new Error("host unavailable");
    }),
    mountStartupFailure: vi.fn(() => ({ unmount: unmountFailure })),
  });
  const renderer = new ClientRenderer(deps);
  await expect(renderer.start()).rejects.toThrow("host unavailable");
  expect(deps.mount).not.toHaveBeenCalled();
  expect(deps.mountStartupFailure).toHaveBeenCalledExactlyOnceWith(new Error("host unavailable"));
  expect(unmountFailure).not.toHaveBeenCalled();
  expect(deps.closeConnection).toHaveBeenCalledOnce();
  await renderer.dispose();
  await renderer.dispose();
  expect(unmountFailure).toHaveBeenCalledOnce();
  expect(deps.closeConnection).toHaveBeenCalledOnce();
});

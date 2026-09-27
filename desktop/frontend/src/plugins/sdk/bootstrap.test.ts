import type { Host } from "dougong";
import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createKernel, startKernel, stopKernel } from "./bootstrap";
import { definePlugin } from "./definePlugin";
import { publishedKernel, retractKernel, useInstalledPlugins } from "./kernel";

const hosts: Host[] = [];

afterEach(async () => {
  await Promise.allSettled(
    hosts.splice(0).map(async (host) => {
      retractKernel(host);
      await host.stop();
    }),
  );
});

function plugin(name: string) {
  return definePlugin({ name, setup() {} });
}

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((accept) => {
    resolve = accept;
  });
  return { promise, resolve };
}

describe("kernel generation ownership", () => {
  it("publishes only the successor generation's installation read model", async () => {
    hosts.push(await startKernel([plugin("test.old")]));
    hosts.push(await startKernel([plugin("test.successor")]));

    expect(renderHook(() => useInstalledPlugins()).result.current).toEqual([
      "flame.kernel.shell",
      "test.successor",
    ]);
  });

  it("does not let a stale stop retract the successor generation", async () => {
    const old = await startKernel([plugin("test.old")]);
    const successor = await startKernel([plugin("test.successor")]);
    hosts.push(old, successor);

    await stopKernel(old);

    expect(publishedKernel()).toBe(successor);
    expect(renderHook(() => useInstalledPlugins()).result.current).toEqual([
      "flame.kernel.shell",
      "test.successor",
    ]);
  });

  it("keeps unpublished installations isolated from the active generation", async () => {
    const active = await startKernel([plugin("test.active")]);
    const unpublished = createKernel([plugin("test.unpublished")]);
    hosts.push(active, unpublished);

    expect(publishedKernel()).toBe(active);
    expect(renderHook(() => useInstalledPlugins()).result.current).toEqual([
      "flame.kernel.shell",
      "test.active",
    ]);
  });

  it("follows Host installation and removal without a second registration path", async () => {
    const host = await startKernel([plugin("test.active")]);
    hosts.push(host);
    const hook = renderHook(() => useInstalledPlugins());
    const change = host.change();
    const late = change.install(plugin("test.late"));

    await act(() => change.commit());

    expect(hook.result.current).toEqual(["flame.kernel.shell", "test.active", "test.late"]);

    await act(() => late.remove());

    expect(hook.result.current).toEqual(["flame.kernel.shell", "test.active"]);
    hook.unmount();
  });

  it("reads the restored Host plan after a failed installation", async () => {
    const host = await startKernel([plugin("test.active")]);
    hosts.push(host);
    const hook = renderHook(() => useInstalledPlugins());
    const change = host.change();
    change.install(
      definePlugin({
        name: "test.failed",
        setup() {
          throw new Error("setup failed");
        },
      }),
    );

    await act(async () => {
      await expect(change.commit()).rejects.toThrow("setup failed");
    });

    expect(hook.result.current).toEqual(["flame.kernel.shell", "test.active"]);
    hook.unmount();
  });

  it("rolls back a startup that was retired before its non-cooperative setup settled", async () => {
    const setup = deferred();
    const cleanup = vi.fn();
    const controller = new AbortController();
    const starting = startKernel(
      [
        definePlugin<{}, {}>({
          name: "test.delayed",
          async setup(ctx) {
            ctx.cleanup(cleanup);
            await setup.promise;
          },
        }),
      ],
      controller.signal,
    );

    controller.abort();
    setup.resolve();

    await expect(starting).rejects.toThrow(/aborted/i);
    expect(cleanup).toHaveBeenCalledOnce();
    expect(publishedKernel()).toBeUndefined();
    expect(renderHook(() => useInstalledPlugins()).result.current).toEqual([]);
  });
});

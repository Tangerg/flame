import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { type Host } from "dougong";
import { act } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createKernel, startKernel, stopKernel } from "@/extensions/bootstrap";
import { definePlugin } from "@/extensions/definePlugin";
import { publishedKernel, retractKernel, useInstalledPlugins } from "@/extensions/kernel";
const hosts: Host[] = [];

afterEach(async () => {
  await Promise.allSettled(
    hosts.splice(0).map(async (host) => {
      await retractKernel(host);
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

    expect(testClient.renderHook(() => useInstalledPlugins()).result.current).toEqual([
      "test.successor",
    ]);
  });

  it("does not let a stale stop retract the successor generation", async () => {
    const old = await startKernel([plugin("test.old")]);
    const successor = await startKernel([plugin("test.successor")]);
    hosts.push(old, successor);

    await stopKernel(old);

    expect(publishedKernel()).toBe(successor);
    expect(testClient.renderHook(() => useInstalledPlugins()).result.current).toEqual([
      "test.successor",
    ]);
  });

  it("keeps unpublished installations isolated from the active generation", async () => {
    const active = await startKernel([plugin("test.active")]);
    const unpublished = createKernel([plugin("test.unpublished")]);
    hosts.push(active, unpublished);

    expect(publishedKernel()).toBe(active);
    expect(testClient.renderHook(() => useInstalledPlugins()).result.current).toEqual([
      "test.active",
    ]);
  });

  it("follows Host installation and removal without a second registration path", async () => {
    const host = await startKernel([plugin("test.active")]);
    hosts.push(host);
    const hook = testClient.renderHook(() => useInstalledPlugins());
    const change = host.change();
    const late = change.install(plugin("test.late"), { id: "test.late" });

    await act(async () => host.apply(await host.plan(change)));

    expect(hook.result.current).toEqual(["test.active", "test.late"]);

    const removal = host.change();
    removal.remove(late);
    await act(async () => host.apply(await host.plan(removal)));

    expect(hook.result.current).toEqual(["test.active"]);
    hook.unmount();
  });

  it("reads the restored Host plan after a failed installation", async () => {
    const host = await startKernel([plugin("test.active")]);
    hosts.push(host);
    const hook = testClient.renderHook(() => useInstalledPlugins());
    const change = host.change();
    change.install(
      definePlugin({
        name: "test.failed",
        setup() {
          throw new Error("setup failed");
        },
      }),
      { id: "test.failed" },
    );

    await act(async () => {
      await expect(host.apply(await host.plan(change))).rejects.toMatchObject({
        cause: expect.objectContaining({ message: "setup failed" }),
        details: { outcome: "restored" },
      });
    });

    expect(hook.result.current).toEqual(["test.active"]);
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
            ctx.lifetime.defer(cleanup);
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
    expect(testClient.renderHook(() => useInstalledPlugins()).result.current).toEqual([]);
  });
});

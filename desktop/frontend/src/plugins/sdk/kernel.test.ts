import { asyncDisposeSymbol } from "dougong";
import { createHost, type AnyPlugin, type Host } from "dougong";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineExtensionPoint } from "./contracts";
import { contributionsTo, publishKernel, retractKernel, subscribeContributions } from "./kernel";
import { definePlugin, type PluginContext } from "./definePlugin";
import { usePluginErrorStore } from "./errors";

interface Theme {
  id: string;
  label: string;
  order?: number;
}

const THEME = defineExtensionPoint<Theme>({ id: "test.theme", keying: "single" });

let host: Host | undefined;

afterEach(async () => {
  if (host) {
    retractKernel(host);
    await host.stop();
  }
  host = undefined;
});

function stand(plugins: AnyPlugin[]): Host {
  const next = createHost({ name: "test", onError: () => {} });
  for (const plugin of plugins) next.install(plugin);
  return next;
}

const index = { entries: contributionsTo };

async function start(plugins: AnyPlugin[]) {
  host = stand(plugins);
  await host.start();
  publishKernel(host);
  return index;
}

describe("plugin declaration ownership", () => {
  it("captures setup and attributes contributions to the installed identity", async () => {
    const setup = vi.fn((ctx: PluginContext) => {
      ctx.contribute(THEME, { id: "declared", label: "Declared" });
    });
    const spec = { name: "test.declared", setup };
    const plugin = definePlugin(spec);
    spec.name = "test.replacement";
    spec.setup = vi.fn(() => {
      throw new Error("replaced setup must not run");
    });

    const index = await start([plugin]);

    expect(setup).toHaveBeenCalledOnce();
    expect(index.entries(THEME)).toEqual([
      expect.objectContaining({
        plugin: "test.declared",
        item: { id: "declared", label: "Declared" },
      }),
    ]);
  });

  it("rejects accessor declarations without executing their getters", () => {
    const setup = vi.fn(() => () => {});
    const spec = {
      name: "test.accessor",
      get setup() {
        return setup();
      },
    };

    expect(() => definePlugin(spec)).toThrow(TypeError);
    expect(setup).not.toHaveBeenCalled();
  });

  it("rejects a non-callable setup before creating an installation", () => {
    const spec = { name: "test.invalid", setup() {} };
    Reflect.set(spec, "setup", null);

    expect(() => definePlugin(spec)).toThrow(TypeError);
  });
});

describe("kernel contribution reads", () => {
  it("reports a failing observer while still notifying the remaining consumers", async () => {
    await start([]);
    const failure = new Error("contribution observer failed");
    const failed = subscribeContributions(() => {
      throw failure;
    });
    const survivor = vi.fn();
    const survived = subscribeContributions(survivor);
    try {
      expect(() => publishKernel(host!)).not.toThrow();
      expect(survivor).toHaveBeenCalledOnce();
      expect(usePluginErrorStore.getState().log).toContainEqual(
        expect.objectContaining({ plugin: "kernel", source: "events", message: failure.message }),
      );
    } finally {
      failed();
      survived();
    }
  });

  it("publishes a contribution under its domain key, not Core's owner-qualified one", async () => {
    const contributor = definePlugin({
      name: "test.contributor",
      setup: (ctx) => {
        ctx.contribute(THEME, { id: "dark", label: "Dark" });
      },
    });

    const index = await start([contributor]);

    expect(index.entries(THEME).map((e) => e.key)).toEqual(["dark"]);
    expect(index.entries(THEME)[0]?.plugin).toBe("test.contributor");
  });

  it("lets the same Host lifetime retire a package's nested contributions", async () => {
    let owned: import("./definePlugin").ContributionLifetime | undefined;
    const index = await start([
      definePlugin({
        name: "test.package",
        setup(ctx) {
          owned = ctx.lifetime("release");
          owned.lifetime("view").contribute(THEME, { id: "package-theme", label: "Package" });
        },
      }),
    ]);
    expect(index.entries(THEME).map((entry) => entry.key)).toEqual(["package-theme"]);
    await owned![asyncDisposeSymbol]();
    expect(owned!.signal.aborted).toBe(true);
    expect(index.entries(THEME)).toEqual([]);
  });

  it("sorts by the contribution value's current order", async () => {
    let update!: (theme: Theme) => void;
    const plugin = definePlugin({
      name: "test.many",
      setup: (ctx) => {
        ctx.contribute(THEME, { id: "c", label: "C", order: 30 });
        update = ctx.contribute(THEME, { id: "a", label: "A", order: 1 }).update;
        ctx.contribute(THEME, { id: "b", label: "B", order: 20 });
      },
    });

    const index = await start([plugin]);

    expect(index.entries(THEME).map((e) => e.item.id)).toEqual(["a", "b", "c"]);
    update({ id: "a", label: "A", order: 40 });
    expect(index.entries(THEME).map((e) => e.item.id)).toEqual(["b", "c", "a"]);
  });

  it("gives a single point's key to the last contributor", async () => {
    const base = definePlugin({
      name: "test.base",
      setup: (ctx) => {
        ctx.contribute(THEME, { id: "dark", label: "Base" });
      },
    });
    const override = definePlugin({
      name: "test.override",
      setup: (ctx) => {
        ctx.contribute(THEME, { id: "dark", label: "Override" });
      },
    });

    const index = await start([base, override]);

    expect(index.entries(THEME).map((e) => e.item.label)).toEqual(["Override"]);
  });

  it("keeps every contribution to a multi point, including one plugin's duplicates", async () => {
    const HANDLER = defineExtensionPoint<{ run: () => void }>({
      id: "test.handler",
      keying: "multi",
    });
    const plugin = definePlugin({
      name: "test.handlers",
      setup: (ctx) => {
        ctx.contribute(HANDLER, { run: () => {} });
        ctx.contribute(HANDLER, { run: () => {} });
      },
    });

    const index = await start([plugin]);

    expect(index.entries(HANDLER)).toHaveLength(2);
  });

  it("returns the same array reference until something changes", async () => {
    const plugin = definePlugin({
      name: "test.stable",
      setup: (ctx) => {
        ctx.contribute(THEME, { id: "dark", label: "Dark" });
      },
    });

    const index = await start([plugin]);

    expect(index.entries(THEME)).toBe(index.entries(THEME));
  });

  it("reads empty for a point nothing contributed to, without minting a new array", async () => {
    const UNDECLARED = defineExtensionPoint<Theme>({ id: "test.undeclared", keying: "single" });

    const index = await start([]);

    expect(index.entries(UNDECLARED)).toEqual([]);
    expect(index.entries(UNDECLARED)).toBe(index.entries(UNDECLARED));
  });

  it("notifies subscribers when a later change adds a contribution", async () => {
    const base = definePlugin({
      name: "test.base",
      setup: (ctx) => {
        ctx.contribute(THEME, { id: "dark", label: "Base" });
      },
    });
    const late = definePlugin({
      name: "test.late",
      setup: (ctx) => {
        ctx.contribute(THEME, { id: "light", label: "Light" });
      },
    });

    const index = await start([base]);
    let notified = 0;
    const stop = subscribeContributions(() => notified++);
    index.entries(THEME);

    const change = host!.change();
    change.install(late);
    await change.commit();

    expect(notified).toBeGreaterThan(0);
    expect(index.entries(THEME).map((e) => e.key)).toEqual(["dark", "light"]);
    stop();
  });

  it("restores a shadowed contribution when the plugin shadowing it is removed", async () => {
    const base = definePlugin({
      name: "test.base",
      setup: (ctx) => {
        ctx.contribute(THEME, { id: "dark", label: "Base" });
      },
    });
    const override = definePlugin({
      name: "test.override",
      setup: (ctx) => {
        ctx.contribute(THEME, { id: "dark", label: "Override" });
      },
    });

    host = stand([base]);
    const installed = host.install(override);
    await host.start();
    publishKernel(host);
    expect(index.entries(THEME).map((e) => e.item.label)).toEqual(["Override"]);

    await installed.remove();

    expect(index.entries(THEME).map((e) => e.item.label)).toEqual(["Base"]);
  });

  it("updates the contribution value without changing its ownership or overriding a later entry", async () => {
    let update!: (theme: Theme) => void;
    const base = definePlugin({
      name: "test.updatable-base",
      setup(ctx) {
        update = ctx.contribute(THEME, { id: "dark", label: "Base" }).update;
      },
    });
    const override = definePlugin({
      name: "test.update-override",
      setup(ctx) {
        ctx.contribute(THEME, { id: "dark", label: "Override" });
      },
    });
    host = stand([base]);
    const installed = host.install(override);
    await host.start();
    publishKernel(host);

    update({ id: "dark", label: "Updated base" });
    expect(index.entries(THEME).map((entry) => entry.item.label)).toEqual(["Override"]);
    expect(() => update({ id: "light", label: "Changed identity" })).toThrow(
      "cannot change its key",
    );

    await installed.remove();

    expect(index.entries(THEME).map((entry) => entry.item.label)).toEqual(["Updated base"]);
    await host.stop();
    expect(() => update({ id: "dark", label: "Late update" })).toThrow(/disposed/i);
  });
});

describe("contribute policy", () => {
  it("keeps a contribution bound to the options accepted at registration", async () => {
    const options = { key: "accepted" };
    let update!: (theme: Theme) => void;
    await start([
      definePlugin({
        name: "test.captured-options",
        setup(ctx) {
          update = ctx.contribute(THEME, { id: "theme", label: "Original" }, options).update;
        },
      }),
    ]);
    options.key = "replacement";

    expect(() => update({ id: "theme", label: "Updated" })).not.toThrow();
    expect(index.entries(THEME)).toEqual([
      expect.objectContaining({ key: "accepted", item: { id: "theme", label: "Updated" } }),
    ]);
  });

  it("keeps extension identity and resolution policy immutable", () => {
    const point = defineExtensionPoint<Theme>({ id: "test.fixed-policy", keying: "single" });

    expect(Reflect.set(point, "id", "test.other-policy")).toBe(false);
    expect(Reflect.set(point, "keying", "multi")).toBe(false);
    expect(point.id).toBe(point.token.id);
  });

  it("derives a single point's key from keyOf ahead of item.id", async () => {
    const ICON = defineExtensionPoint<{ id: string; fn: string }>({
      id: "test.icon",
      keying: "single",
      keyOf: (item) => item.fn,
    });
    const plugin = definePlugin({
      name: "test.icons",
      setup: (ctx) => {
        ctx.contribute(ICON, { id: "ignored", fn: "read_file" });
      },
    });

    const index = await start([plugin]);

    expect(index.entries(ICON)[0]?.key).toBe("read_file");
  });

  it("normalizes a key so a registration and a lookup of the same combo agree", async () => {
    const SLASH = defineExtensionPoint<{ label: string }>({
      id: "test.slash",
      keying: "single",
      normalizeKey: (key) => (key.startsWith("/") ? key : `/${key}`),
    });
    const plugin = definePlugin({
      name: "test.slashes",
      setup: (ctx) => {
        ctx.contribute(SLASH, { label: "Ping" }, { key: "ping" });
      },
    });

    const index = await start([plugin]);

    expect(index.entries(SLASH)[0]?.key).toBe("/ping");
  });

  it("refuses a single contribution with no key to be found anywhere", async () => {
    const KEYLESS = defineExtensionPoint<{ label: string }>({
      id: "test.keyless",
      keying: "single",
    });
    const plugin = definePlugin({
      name: "test.keyless",
      setup: (ctx) => {
        ctx.contribute(KEYLESS, { label: "no id" });
      },
    });

    host = stand([plugin]);

    await expect(host.start()).rejects.toThrow(/requires opts.key, keyOf, or a non-empty item.id/);
  });
});

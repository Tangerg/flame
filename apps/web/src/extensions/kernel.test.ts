import { asyncDisposeSymbol } from "dougong";
import { createHost, type AnyPlugin, type Host } from "dougong";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineExtensionPoint } from "@/extensions/contracts";
import {
  contributionsTo,
  publishKernel,
  retractKernel,
  subscribeContributions,
} from "@/extensions/kernel";
import { definePlugin, type PluginContext } from "@/extensions/definePlugin";
import { usePluginErrorStore } from "@/extensions/errors";
interface Theme {
  id: string;
  label: string;
  order?: number;
}

const THEME = defineExtensionPoint<Theme>({ id: "test.theme", keying: "single" });

let host: Host | undefined;

afterEach(async () => {
  if (host) {
    await retractKernel(host);
    await host.stop();
  }
  host = undefined;
});

function stand(plugins: AnyPlugin[]): Host {
  return createHost({
    name: "test",
    onError: () => {},
    installations: plugins.map((plugin) => ({ id: plugin.name, plugin })),
  });
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
      ctx.contributions.add(THEME, { id: "declared", label: "Declared" });
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
        ctx.contributions.add(THEME, { id: "dark", label: "Dark" });
      },
    });

    const index = await start([contributor]);

    expect(index.entries(THEME).map((e) => e.key)).toEqual(["dark"]);
    expect(index.entries(THEME)[0]?.plugin).toBe("test.contributor");
  });

  it("lets the same Host lifetime retire a package's nested contributions", async () => {
    let owned: import("@/extensions/definePlugin").ContributionScope | undefined;
    const index = await start([
      definePlugin({
        name: "test.package",
        setup(ctx) {
          owned = ctx.contributions.child("release");
          owned.child("view").add(THEME, { id: "package-theme", label: "Package" });
        },
      }),
    ]);
    expect(index.entries(THEME).map((entry) => entry.key)).toEqual(["package-theme"]);
    await owned![asyncDisposeSymbol]();
    expect(owned!.lifetime.signal.aborted).toBe(true);
    expect(index.entries(THEME)).toEqual([]);
  });

  it("sorts by the contribution value's current order", async () => {
    let update!: (theme: Theme) => void;
    const plugin = definePlugin({
      name: "test.many",
      setup: (ctx) => {
        ctx.contributions.add(THEME, { id: "c", label: "C", order: 30 });
        update = ctx.contributions.add(THEME, { id: "a", label: "A", order: 1 }).update;
        ctx.contributions.add(THEME, { id: "b", label: "B", order: 20 });
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
        ctx.contributions.add(THEME, { id: "dark", label: "Base" });
      },
    });
    const override = definePlugin({
      name: "test.override",
      setup: (ctx) => {
        ctx.contributions.add(THEME, { id: "dark", label: "Override" });
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
        ctx.contributions.add(HANDLER, { run: () => {} });
        ctx.contributions.add(HANDLER, { run: () => {} });
      },
    });

    const index = await start([plugin]);

    expect(index.entries(HANDLER)).toHaveLength(2);
  });

  it("returns the same array reference until something changes", async () => {
    const plugin = definePlugin({
      name: "test.stable",
      setup: (ctx) => {
        ctx.contributions.add(THEME, { id: "dark", label: "Dark" });
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
        ctx.contributions.add(THEME, { id: "dark", label: "Base" });
      },
    });
    const late = definePlugin({
      name: "test.late",
      setup: (ctx) => {
        ctx.contributions.add(THEME, { id: "light", label: "Light" });
      },
    });

    const index = await start([base]);
    let notified = 0;
    const stop = subscribeContributions(() => notified++);
    index.entries(THEME);

    const change = host!.change();
    change.install(late, { id: late.name });
    await retractKernel(host!);
    await host!.apply(await host!.plan(change));
    publishKernel(host!);

    expect(notified).toBeGreaterThan(0);
    expect(index.entries(THEME).map((e) => e.key)).toEqual(["dark", "light"]);
    stop();
  });

  it("restores a shadowed contribution when the plugin shadowing it is removed", async () => {
    const base = definePlugin({
      name: "test.base",
      setup: (ctx) => {
        ctx.contributions.add(THEME, { id: "dark", label: "Base" });
      },
    });
    const override = definePlugin({
      name: "test.override",
      setup: (ctx) => {
        ctx.contributions.add(THEME, { id: "dark", label: "Override" });
      },
    });

    host = stand([base]);
    const installation = host.change();
    const installed = installation.install(override, { id: override.name });
    await host.apply(await host.plan(installation));
    await host.start();
    publishKernel(host);
    expect(index.entries(THEME).map((e) => e.item.label)).toEqual(["Override"]);

    const removal = host.change();
    removal.remove(installed);
    await retractKernel(host);
    await host.apply(await host.plan(removal));
    publishKernel(host);

    expect(index.entries(THEME).map((e) => e.item.label)).toEqual(["Base"]);
  });

  it("updates the contribution value without changing its ownership or overriding a later entry", async () => {
    let update!: (theme: Theme) => void;
    const base = definePlugin({
      name: "test.updatable-base",
      setup(ctx) {
        update = ctx.contributions.add(THEME, { id: "dark", label: "Base" }).update;
      },
    });
    const override = definePlugin({
      name: "test.update-override",
      setup(ctx) {
        ctx.contributions.add(THEME, { id: "dark", label: "Override" });
      },
    });
    host = stand([base]);
    const installation = host.change();
    const installed = installation.install(override, { id: override.name });
    await host.apply(await host.plan(installation));
    await host.start();
    publishKernel(host);

    update({ id: "dark", label: "Updated base" });
    expect(index.entries(THEME).map((entry) => entry.item.label)).toEqual(["Override"]);
    expect(() => update({ id: "light", label: "Changed identity" })).toThrow(
      "cannot change its key",
    );

    const removal = host.change();
    removal.remove(installed);
    await retractKernel(host);
    await host.apply(await host.plan(removal));
    publishKernel(host);

    expect(index.entries(THEME).map((entry) => entry.item.label)).toEqual(["Updated base"]);
    await retractKernel(host);
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
          update = ctx.contributions.add(THEME, { id: "theme", label: "Original" }, options).update;
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
        ctx.contributions.add(ICON, { id: "ignored", fn: "read_file" });
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
        ctx.contributions.add(SLASH, { label: "Ping" }, { key: "ping" });
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
        ctx.contributions.add(KEYLESS, { label: "no id" });
      },
    });

    host = stand([plugin]);

    await expect(host.start()).rejects.toMatchObject({
      cause: expect.objectContaining({
        message: expect.stringContaining("requires a non-empty key"),
      }),
      details: { outcome: "restored" },
    });
  });
});

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { builtinPlugins } from "./index";
import * as kernelPoints from "@/plugins/sdk/kernelPoints";
import { publishedKernel } from "@/plugins/sdk/kernel";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";

/**
 * No socket. Loading every built-in starts the plugins that talk to the Runtime, and they
 * reach for the default endpoint — so without this the suite's result depends on whether
 * something happens to be listening on 17171, which on a developer's machine it often is.
 */
beforeEach(() => {
  vi.stubGlobal("fetch", () => Promise.reject(new Error("offline in tests")));
  vi.stubGlobal(
    "EventSource",
    class {
      close() {}
      addEventListener() {}
    },
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("built-in plugin manifest", () => {
  it("declares every plugin identity exactly once", () => {
    const names = builtinPlugins.map((plugin) => plugin.name);
    expect(new Set(names).size).toBe(names.length);
  });

  it("keeps cumulative context telemetry out of the title bar", () => {
    const names = builtinPlugins.map((plugin) => plugin.name);
    expect(names).not.toContain("flame.builtin.session-usage");
  });
});

/**
 * On a `single` point, two contributions under one key means the second shadows the first.
 *
 * That is the mechanism, and it is deliberate: `keying` is a READ policy, so an override
 * comes back when the overriding plugin unloads. It is the right shape for a third-party
 * plugin replacing a default — and always a mistake between two BUILT-INS, which ship as one
 * product and cannot be meaning to override each other. The kernel resolves the pair before
 * anything outside it can look, so the loser leaves no trace: `contributionsTo` already
 * returns one entry per key.
 *
 * The raw view is what the resolution is done over, so this reads that instead. Twenty-five
 * points, and the plugin that lost is named rather than merely counted.
 */
describe("built-in contributions", () => {
  afterEach(async () => {
    await resetKernelForTest();
  });

  it("never have two built-ins under one key on a single-keyed point", async () => {
    await loadPluginsForTest(...builtinPlugins);
    const host = publishedKernel();
    expect(host, "the test kernel published nothing").toBeDefined();

    const shadowed: string[] = [];
    let contributions = 0;
    for (const [name, candidate] of Object.entries(kernelPoints)) {
      const point = candidate as { id?: string; token?: unknown; keying?: string };
      if (!point.token || !point.id || point.keying === "multi") continue;
      const raw = [
        ...(host!
          .contributions(point.token as never)
          .get()
          .values() as Iterable<{
          key: string;
          plugin: string;
        }>),
      ];
      contributions += raw.length;
      const owners = new Map<string, string[]>();
      for (const entry of raw)
        owners.set(entry.key, [...(owners.get(entry.key) ?? []), entry.plugin]);
      for (const [key, plugins] of owners)
        if (plugins.length > 1) shadowed.push(`${name}[${key}] <- ${plugins.join(", ")}`);
    }

    // A kernel that installed nothing shadows nothing, and would pass this silently.
    expect(contributions).toBeGreaterThan(100);
    expect(shadowed).toEqual([]);
  });
});

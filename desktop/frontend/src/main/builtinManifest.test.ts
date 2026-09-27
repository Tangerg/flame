import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createBuiltinPlugins } from "./builtinPlugins";
import { createRuntimeConnection } from "./runtimeConnection";
import { createBrowserHost } from "@/platform/browserHost";
import * as kernelPoints from "@/plugins/sdk/kernelPoints";
import { publishedKernel } from "@/plugins/sdk/kernel";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { installedRuntimeMutationJournalStorage } from "@/plugins/builtin/runtime/public/mutationJournal";
import type { FlameClient } from "@flame/runtime-contract/client";

let connection: ReturnType<typeof createRuntimeConnection>;
const host = createBrowserHost();
let builtinPlugins: ReturnType<typeof createBuiltinPlugins>;

beforeEach(async () => {
  connection = createRuntimeConnection(host);
  await connection.initialize();
  builtinPlugins = createBuiltinPlugins(connection, host);
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

describe("built-in contributions", () => {
  afterEach(async () => {
    await resetKernelForTest();
    await connection.dispose();
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

    expect(contributions).toBeGreaterThan(100);
    expect(shadowed).toEqual([]);
  });

  // Every Runtime adapter must compose after the endpoint and mutation journal are installed.
  it("hands every startup plugin the client the Runtime plugin finished configuring", async () => {
    const journalInstalled: boolean[] = [];
    vi.spyOn(connection, "client").mockImplementation(() => {
      journalInstalled.push(installedRuntimeMutationJournalStorage() !== null);
      return {} as unknown as FlameClient;
    });
    builtinPlugins = createBuiltinPlugins(connection, host);

    await loadPluginsForTest(...builtinPlugins);

    expect(journalInstalled.length).toBeGreaterThan(0);
    expect(
      journalInstalled.filter((installed) => !installed).length,
      "a plugin composed against the connection before the Runtime plugin configured it",
    ).toBe(0);
  });
});

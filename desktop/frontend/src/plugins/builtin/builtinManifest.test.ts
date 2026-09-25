import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { builtinPlugins } from "./index";
import * as kernelPoints from "@/plugins/sdk/kernelPoints";
import { publishedKernel } from "@/plugins/sdk/kernel";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { installedRuntimeMutationJournalStorage } from "@/plugins/builtin/runtime/public/mutationJournal";
import { resetContainer, setContainer } from "@/main/container";
import type { FlameClient } from "@/rpc";

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

describe("built-in contributions", () => {
  afterEach(async () => {
    await resetKernelForTest();
    await resetContainer();
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

  // The container assembles its client out of what the Runtime plugin installs —
  // the endpoint and the mutation journal. Dougong orders setup by `requires`, so
  // a plugin that composes against the container without declaring that
  // dependency runs first, binds a half-built client, and keeps it after the
  // container retires the incomplete one. That is invisible until a mutation
  // leaves through the dead client: every skill approval answered `client closed`
  // this way. Assert the ordering rather than each plugin's declaration, so the
  // next plugin that forgets fails here instead of in a user's hands.
  it("hands every startup plugin the client the Runtime plugin finished configuring", async () => {
    const journalInstalled: boolean[] = [];
    setContainer({
      client: () => {
        journalInstalled.push(installedRuntimeMutationJournalStorage() !== null);
        return {} as unknown as FlameClient;
      },
    });

    await loadPluginsForTest(...builtinPlugins);

    expect(journalInstalled.length).toBeGreaterThan(0);
    expect(
      journalInstalled.filter((installed) => !installed).length,
      "a plugin composed against the container before the Runtime plugin configured it",
    ).toBe(0);
  });
});

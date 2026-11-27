import { QueryObserver } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import { HTTP_HEADERS, type PluginInstallation } from "@flame/runtime-client/wire";
import { startKernel, stopKernel } from "@/extensions/bootstrap";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { definePlugin } from "@/extensions/definePlugin";
import { contributionsTo } from "@/extensions/kernel";
import { RuntimeConnectionGeneration } from "@/features/connection/public/services";
import { ComponentModules } from "@/features/plugins/adapters/componentModules";
import { createInstalledPackageState, PACKAGES_KEY } from "@/features/plugins/adapters/packages";
import { ProviderSelectionCommands } from "@/features/plugins/adapters/providerSelectionCommands";
import { registerPackageContributions } from "@/features/plugins/contributions";
import { createAppearanceStore } from "@/features/theme/adapters/appearanceStore";
import { createWorkspace } from "@/features/workspace/adapters/workspace";
import { createAppearanceProjection } from "@/lib/appearance";
import { createMemoryNavigator } from "@/lib/navigation.testkit";
import { queryClient } from "@/lib/queryClient";
import { useRuntimeFixture } from "./fixture";
import { within } from "./observation";
import { openWebSession } from "./webSessionFixture";

describe("Go Runtime ↔ HTTP ↔ Web: installed package catalogs", () => {
  const fixture = useRuntimeFixture("plugin-catalog");

  it("checks one original provider selection and refreshes only after confirmation", async () => {
    const provider = (await fixture.client.plugins.listProviders()).data.find(
      ({ role }) => role === "memory",
    );
    if (!provider) throw new Error("Runtime did not select its bundled Memory provider");
    const request = {
      role: provider.role,
      installationId: provider.installationId,
      contribution: provider.contribution,
    };
    const checking = Promise.withResolvers<void>();
    const releaseCheck = Promise.withResolvers<void>();
    const commandKeys = new Set<string | null>();
    let loseOriginal = true;
    let refreshes = 0;
    const client = fixture.createRuntimeClient(async (url, init) => {
      const headers = new Headers(init?.headers);
      if (headers.get(HTTP_HEADERS.method) !== "plugins.selectProvider") return fetch(url, init);
      commandKeys.add(headers.get(HTTP_HEADERS.idempotencyKey));
      if (!loseOriginal) {
        checking.resolve();
        await releaseCheck.promise;
        return fetch(url, init);
      }
      const response = await fetch(url, init);
      await response.arrayBuffer();
      throw new TypeError("the original provider selection response was lost");
    });
    const owner = new AbortController();
    const commands = new ProviderSelectionCommands({
      signal: owner.signal,
      select: (input) => client.plugins.selectProvider(input, owner.signal),
      retry: (original) => original.retry({ signal: owner.signal }),
      refreshCatalog: () => {
        refreshes++;
      },
    });
    try {
      await expect(commands.select(request)).resolves.toBe(false);
      const pending = commands.snapshot(provider.role);
      if (pending?.kind !== "unconfirmed") throw new Error("original selection was not retained");
      expect(refreshes).toBe(0);
      loseOriginal = false;
      const recovery = commands.check(provider.role, pending);
      await within(checking.promise, "the original selection check");
      await expect(commands.check(provider.role, pending)).resolves.toBe(false);
      await expect(commands.select(request)).resolves.toBe(false);
      releaseCheck.resolve();
      await expect(within(recovery, "the accepted provider selection")).resolves.toBe(true);
      await expect(commands.check(provider.role, pending)).resolves.toBe(false);
      expect(commandKeys).toEqual(new Set([pending.idempotencyKey]));
      expect(commands.snapshot(provider.role)).toBeUndefined();
      expect(refreshes).toBe(1);
      await expect(fixture.client.plugins.listProviders()).resolves.toMatchObject({
        data: expect.arrayContaining([expect.objectContaining(request)]),
      });
    } finally {
      releaseCheck.resolve();
      owner.abort();
      await client.close();
    }
  }, 30_000);

  it("does not publish a stale catalog after a newer settings query", async () => {
    const initial = (await fixture.client.plugins.list()).data.find(
      (row) => row.selected.name === "flame.trajectory" && row.enabled,
    );
    if (!initial) throw new Error("the bundled trajectory package is not enabled");
    const oldRead = Promise.withResolvers<void>();
    const release = Promise.withResolvers<void>();
    let heldFirstRead = false;
    const client = fixture.createRuntimeClient(async (url, init) => {
      const isCatalog = new Headers(init?.headers).get(HTTP_HEADERS.method) === "plugins.list";
      const response = await fetch(url, init);
      if (isCatalog && !heldFirstRead) {
        heldFirstRead = true;
        oldRead.resolve();
        await release.promise;
      }
      return response;
    });
    const endpoint = () => fixture.baseUrl;
    const web = openWebSession(client, endpoint(), "");
    const workspace = createWorkspace(
      () => client,
      createMemoryNavigator(),
      { openPath: async () => false, revealPath: async () => false },
      () => false,
      web.agent.access,
      web.runtime,
      web.notifications,
    );
    const appearance = createAppearanceStore(() => localStorage);
    const packages = createInstalledPackageState(endpoint);
    const realizedChange = Promise.withResolvers<void>();
    let observations = 0;
    const stopObserving = packages.workflows.subscribe(() => {
      if (++observations === 2) realizedChange.resolve();
    });
    const modules = new ComponentModules();
    const generation = RuntimeConnectionGeneration.forProcess("package-catalog");
    const host = await startKernel([
      definePlugin({
        name: "flame.test.installed-package-catalog",
        setup(ctx) {
          registerPackageContributions(
            packages,
            ctx.contributions,
            client,
            modules,
            generation,
            endpoint(),
            workspace.access,
            {
              preference: {
                read: appearance.getState,
                edit: appearance.getState,
                subscribe: appearance.subscribe,
              },
              appearance: createAppearanceProjection(),
              system: { scheme: () => "light", subscribe: () => () => {} },
              fonts: { isAvailable: () => false, hasTabularFigures: () => false },
            },
            async () => {
              throw new Error("the test does not save files");
            },
          );
        },
      }),
    ]);
    const queryKey = [PACKAGES_KEY];
    try {
      await within(oldRead.promise, "the original realization catalog read");
      await fixture.client.plugins.setEnablement({ installationId: initial.id, enabled: false });
      const provider = contributionsTo(DATA_PROVIDER).find(({ key }) => key === PACKAGES_KEY);
      if (!provider) throw new Error("the package query did not register its Runtime provider");
      const options = {
        queryKey,
        queryFn: ({ signal }: { signal: AbortSignal }) =>
          provider.item.fetcher(undefined, signal) as Promise<PluginInstallation[]>,
        staleTime: Infinity,
      };
      const fresh = await queryClient.fetchQuery(options);
      expect(fresh.find(({ id }) => id === initial.id)?.enabled).toBe(false);
      const observed: boolean[] = [];
      const observer = new QueryObserver(queryClient, options);
      const close = observer.subscribe(({ data }) => {
        const installation = data?.find(({ id }) => id === initial.id);
        if (installation) observed.push(installation.enabled);
      });
      try {
        release.resolve();
        await within(realizedChange.promise, "the post-change package realization");
        expect(observed).not.toContain(true);
        expect(observer.getCurrentResult().data?.find(({ id }) => id === initial.id)?.enabled).toBe(
          false,
        );
      } finally {
        close();
      }
    } finally {
      release.resolve();
      stopObserving();
      await stopKernel(host);
      queryClient.removeQueries({ queryKey });
      await modules.close();
      workspace.dispose();
      web.dispose();
      await client.close();
    }
  }, 30_000);
});

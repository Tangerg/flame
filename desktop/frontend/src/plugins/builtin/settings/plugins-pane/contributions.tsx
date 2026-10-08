import type { PluginCarrier } from "@/foundation/pluginCarrier";
import { asyncDisposeSymbol } from "dougong";
import type { FlameClient } from "@flame/runtime-contract/client";
import type { PluginInstallation } from "@flame/runtime-contract/wire";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import { DATA_PROVIDER, WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import { PackageView } from "./ui/PackageView";
import { queryClient } from "@/lib/queryClient";
import {
  contributePaletteTheme,
  retainThemeSelection,
} from "@/plugins/builtin/theme/public/appearance";
import { packageOperations, PACKAGES_KEY, usePackageRealization } from "./application/packages";

const packageThemePrefix = "package:";

function packageThemeId(installation: string, theme: string): string {
  return `${packageThemePrefix}${installation}:${theme}`;
}

export function registerPackageContributions(
  scope: ContributionLifetime,
  client: FlameClient,
  carrier: PluginCarrier,
): void {
  scope.cleanup(packageOperations.configure({ ...client.plugins, signal: scope.signal }));
  const fetcher = async (_params?: unknown, signal?: AbortSignal) =>
    (await client.plugins.list(signal ? AbortSignal.any([scope.signal, signal]) : scope.signal))
      .data;
  scope.contribute(DATA_PROVIDER, { key: PACKAGES_KEY, fetcher });
  const resources = new Map<string, { signature: string; lifetime: ContributionLifetime }>();
  const reconcile = async (installations: PluginInstallation[]) => {
    const admitted = installations.filter((item) => item.presentation === "admitted");
    retainThemeSelection(
      admitted.flatMap((item) =>
        item.selected.themes.map((theme) => packageThemeId(item.id, theme.id)),
      ),
      packageThemePrefix,
    );
    const ids = new Set(admitted.map((item) => item.id));
    for (const [id, resource] of resources) {
      if (!ids.has(id)) {
        resources.delete(id);
        await resource.lifetime[asyncDisposeSymbol]();
      }
    }
    for (const installation of admitted) {
      if (scope.signal.aborted) return;
      const release = installation.selected;
      const signature = release.digest;
      const previous = resources.get(installation.id);
      if (previous?.signature === signature) continue;
      if (previous) {
        resources.delete(installation.id);
        await previous.lifetime[asyncDisposeSymbol]();
      }
      if (scope.signal.aborted) return;
      const lifetime = scope.lifetime(installation.id);
      resources.set(installation.id, { signature, lifetime });
      for (const view of release.views) {
        const binding = {
          installationId: installation.id,
          digest: release.digest,
          viewId: view.id,
        };
        const reads = (sessionId: string) => {
          const read = (cursor: string | undefined, signal: AbortSignal) =>
            client.plugins.readTrajectory({ ...binding, sessionId, cursor }, signal);
          return {
            read,
            async load(signal: AbortSignal) {
              const [resource, initial] = await Promise.all([
                client.plugins.readView(binding, signal),
                read(undefined, signal),
              ]);
              return { html: resource.html, initial };
            },
          };
        };
        const title = `${release.name} · ${view.title}`;
        const component = () => (
          <PackageView reads={reads} title={title} lifetime={lifetime} carrier={carrier} />
        );
        lifetime.contribute(WORKSPACE_VIEW, {
          id: `package:${installation.id}:${view.id}`,
          title,
          dock: "session",
          component,
        });
      }
      for (const theme of release.themes) {
        const { background, foreground, accent, muted, border } = theme.colors;
        contributePaletteTheme(lifetime, {
          id: packageThemeId(installation.id, theme.id),
          label: `${release.name} · ${theme.title}`,
          scheme: theme.scheme,
          palette: { background, foreground, accent, muted, border },
        });
      }
    }
  };
  let failure: { reason: string; retry(): void } | null = null;
  const withdrawFailure = () => {
    if (failure && usePackageRealization.getState().failure === failure)
      usePackageRealization.setState({ failure: null });
    failure = null;
  };
  scope.cleanup(withdrawFailure);
  const realize = () => {
    withdrawFailure();
    scope.spawn(async (signal) => {
      const refresh = async () => {
        await queryClient.invalidateQueries({ queryKey: [PACKAGES_KEY], refetchType: "none" });
        const rows = await queryClient.fetchQuery({
          queryKey: [PACKAGES_KEY],
          queryFn: ({ signal: querySignal }) => fetcher(undefined, querySignal),
        });
        if (signal.aborted) return;
        await reconcile(rows);
      };
      try {
        const subscription = await client.runtimeEvents.subscribe(
          { topics: ["plugins.changed"] },
          signal,
        );
        const events = subscription.events[Symbol.asyncIterator]();
        try {
          await refresh();
          while (!(await events.next()).done) await refresh();
        } finally {
          await events.return?.();
        }
      } catch (error) {
        for (const resource of resources.values()) await resource.lifetime[asyncDisposeSymbol]();
        resources.clear();
        if (signal.aborted) return;
        failure = {
          reason: error instanceof Error ? error.message : String(error),
          retry: realize,
        };
        usePackageRealization.setState({ failure });
      }
    });
  };
  realize();
}

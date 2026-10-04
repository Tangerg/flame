import { asyncDisposeSymbol } from "dougong";
import type { FlameClient } from "@flame/runtime-contract/client";
import type { PluginInstallation } from "@flame/runtime-contract/wire";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import { COLOR_THEME, DATA_PROVIDER } from "@/plugins/sdk/kernelPoints";
import { queryClient } from "@/lib/queryClient";
import { retainThemeSelection } from "@/plugins/builtin/theme/public/appearance";
import { packageOperations, PACKAGES_KEY } from "./application/packages";

const themeTokens = new Map([
  ["background", "color-bg"],
  ["foreground", "color-text"],
  ["accent", "color-accent"],
  ["muted", "color-text-muted"],
  ["border", "color-border"],
]);

const packageThemePrefix = "package:";

function packageThemeId(installation: string, theme: string): string {
  return `${packageThemePrefix}${installation}:${theme}`;
}

export function registerPackageContributions(
  scope: ContributionLifetime,
  client: FlameClient,
): void {
  scope.cleanup(packageOperations.configure({ ...client.plugins, signal: scope.signal }));
  const fetcher = async (_params?: unknown, signal?: AbortSignal) =>
    (await client.plugins.list(signal ? AbortSignal.any([scope.signal, signal]) : scope.signal))
      .data;
  scope.contribute(DATA_PROVIDER, { key: PACKAGES_KEY, fetcher });
  const resources = new Map<string, { signature: string; lifetime: ContributionLifetime }>();
  const reconcile = async (installations: PluginInstallation[]) => {
    const allThemes = installations.flatMap((item) =>
      item.selected.themes.map((theme) => packageThemeId(item.id, theme.id)),
    );
    retainThemeSelection(allThemes, packageThemePrefix);
    const admitted = installations.filter(
      (item) =>
        item.enabled &&
        item.approvedDigest === item.selected.digest &&
        !item.availability.some((diagnostic) => diagnostic.component === "release"),
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
      for (const theme of release.themes) {
        const tokens: Record<string, string> = {};
        for (const [key, value] of Object.entries(theme.colors)) {
          const token = themeTokens.get(key);
          if (token) tokens[token] = value;
        }
        lifetime.contribute(COLOR_THEME, {
          id: packageThemeId(installation.id, theme.id),
          label: `${release.name} · ${theme.title}`,
          scheme: theme.scheme,
          tokens,
        });
      }
    }
  };
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
      await refresh();
      for await (const _event of subscription.events) {
        await refresh();
      }
    } catch (error) {
      for (const resource of resources.values()) await resource.lifetime[asyncDisposeSymbol]();
      resources.clear();
      if (!signal.aborted) throw error;
    }
  });
}

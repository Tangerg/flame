import type { PluginCarrier } from "@/foundation/pluginCarrier";
import { asyncDisposeSymbol, isCancellationReason } from "dougong";
import { isCancelledError } from "@tanstack/react-query";
import type { FlameClient } from "@flame/runtime-contract/client";
import type { PluginInstallation } from "@flame/runtime-contract/wire";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import { DATA_PROVIDER, WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import { PackageView } from "./ui/PackageView";
import { queryClient } from "@/lib/queryClient";
import { failureMessage } from "@/lib/diagnostics";
import {
  contributePaletteTheme,
  retainThemeSelection,
} from "@/plugins/builtin/theme/public/appearance";
import { packageOperations, PACKAGES_KEY, usePackageRealization } from "./application/packages";
import { createTrajectoryViewReads } from "./adapters/trajectoryReads";

const packageThemePrefix = "package:";

function packageThemeId(installation: string, theme: string): string {
  return `${packageThemePrefix}${installation}:${theme}`;
}

function createPackageReconciler(
  scope: ContributionLifetime,
  client: FlameClient,
  carrier: PluginCarrier,
): (installations: PluginInstallation[]) => Promise<void> {
  const resources = new Map<string, { signature: string; lifetime: ContributionLifetime }>();
  return async (installations: PluginInstallation[]) => {
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
        const reads = (sessionId: string) =>
          createTrajectoryViewReads(client.plugins, binding, sessionId);
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
  let failure: { reason: string; retry(): void } | null = null;
  const withdrawFailure = () => {
    if (failure && usePackageRealization.getState().failure === failure)
      usePackageRealization.setState({ failure: null });
    failure = null;
  };
  scope.cleanup(withdrawFailure);
  const realize = () => {
    withdrawFailure();
    const contributions = scope.lifetime("package-realization");
    const reconcile = createPackageReconciler(contributions, client, carrier);
    // The generation owns the task so it can join its contribution scope on failure.
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
        if (signal.aborted) {
          await events.return?.();
          return;
        }
        const closeEvents = contributions.cleanup(() => events.return?.());
        await refresh();
        while (!(await events.next()).done) await refresh();
        await closeEvents[asyncDisposeSymbol]();
      } catch (error) {
        const reasons = [error];
        try {
          await contributions[asyncDisposeSymbol]();
        } catch (cleanupError) {
          reasons.push(cleanupError);
        }
        if (signal.aborted) {
          const unexpected = reasons.filter(
            (cause) => !isCancellationReason(signal, cause) && !isCancelledError(cause),
          );
          if (unexpected.length === 1) throw unexpected[0];
          if (unexpected.length > 1)
            throw new AggregateError(unexpected, "Package realization retirement failed");
          return;
        }
        const failed = {
          reason: reasons.map(failureMessage).join("; "),
          retry() {
            if (failure === failed && !scope.signal.aborted) realize();
          },
        };
        failure = failed;
        usePackageRealization.setState({ failure });
      }
    });
  };
  realize();
}

import type { PluginCarrier } from "@/foundation/pluginCarrier";
import { lazy, Suspense } from "react";
import { ComponentModules } from "./adapters/componentModules";
import { InstalledComponent, ComponentBoundary, ComponentLoading } from "./ui/InstalledComponent";
import { currentRuntimeEndpoint } from "@/plugins/builtin/runtime/public/endpoint";
import { asyncDisposeSymbol, isCancellationReason } from "dougong";
import type { FlameClient } from "@flame/runtime-contract/client";
import type { PluginInstallation } from "@flame/runtime-contract/wire";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import {
  DATA_PROVIDER,
  WORKSPACE_VIEW,
  TOOL_RESULT_RENDERER,
  WORKSPACE_FILE_RENDERER,
} from "@/plugins/sdk/kernelPoints";
import { ToolResultPackage } from "./ui/ToolResultPackage";
import { FilePackageRenderer } from "./ui/FilePackageRenderer";
import {
  createRecordedRendererReads,
  type ToolResultSnapshot,
  type FileSnapshot,
} from "./adapters/rendererReads";
import { SchedulePackageView } from "./ui/SchedulePackageView";
import { queryClient } from "@/lib/queryClient";
import { failureMessage } from "@/lib/diagnostics";
import {
  contributePaletteTheme,
  retainThemeSelection,
} from "@/plugins/builtin/theme/public/appearance";
import { trackContributionIO } from "@/plugins/sdk/contributionIO";
import {
  packageOperations,
  PACKAGES_KEY,
  PROVIDERS_KEY,
  usePackageRealization,
} from "./application/packages";
import { createScheduleViewReads } from "./adapters/pluginViewReads";

const packageThemePrefix = "package:";

function packageThemeId(installation: string, theme: string): string {
  return `${packageThemePrefix}${installation}:${theme}`;
}

function createPackageReconciler(
  scope: ContributionLifetime,
  client: FlameClient,
  carrier: PluginCarrier,
  modules: ComponentModules,
  generation: object,
  moduleScope: ContributionLifetime,
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
      for (const entry of release.components) {
        const contribution = release.contributions.find((value) => value.id === entry.contribution);
        if (entry.contribution && !contribution)
          throw new Error("The component's declared callable contribution is unavailable.");
        const binding = contribution
          ? {
              installationId: installation.id,
              digest: release.digest,
              contribution: contribution.id,
              contract: contribution.contract,
            }
          : undefined;
        const View = lazy(async () => {
          const loaded = await modules.component(
            moduleScope,
            client,
            currentRuntimeEndpoint(),
            generation,
            installation,
            entry,
          );
          return {
            default: () => (
              <InstalledComponent
                entry={loaded.component}
                scope={entry.scope}
                resources={loaded.signal}
                runtime={client}
                binding={binding}
                lifetime={lifetime}
              />
            ),
          };
        });
        const id = `package:${installation.id}:${entry.id}`;
        lifetime.contribute(
          WORKSPACE_VIEW,
          {
            id,
            title: `${release.name} · ${entry.title}`,
            material: entry.material,
            dock: entry.scope === "session" ? "session" : undefined,
            component: () => (
              <ComponentBoundary>
                <Suspense fallback={<ComponentLoading />}>
                  <View />
                </Suspense>
              </ComponentBoundary>
            ),
          },
          { key: id },
        );
      }
      for (const renderer of release.renderers) {
        const binding = {
          installationId: installation.id,
          digest: release.digest,
          rendererId: renderer.id,
        };
        const id = `package:${installation.id}:${renderer.id}`;
        const title = `${release.name} · ${renderer.title}`;
        if (renderer.selector.type === "fileExtension") {
          const reads = (file: FileSnapshot["file"]) =>
            createRecordedRendererReads(client.plugins, binding, { format: "flame.file.v1", file });
          lifetime.contribute(
            WORKSPACE_FILE_RENDERER,
            {
              id,
              title,
              extension: renderer.selector.name,
              component: (props) => (
                <FilePackageRenderer
                  {...props}
                  reads={reads}
                  title={title}
                  lifetime={lifetime}
                  carrier={carrier}
                />
              ),
            },
            { key: id },
          );
          continue;
        }
        const reads = (tool: ToolResultSnapshot["tool"]) =>
          createRecordedRendererReads(client.plugins, binding, {
            format: "flame.tool-result.v1",
            tool,
          });
        lifetime.contribute(
          TOOL_RESULT_RENDERER,
          {
            id,
            title,
            selector: { ...renderer.selector },
            component: ({ tool, fallback }) => (
              <ToolResultPackage
                tool={tool}
                fallback={fallback}
                reads={reads}
                title={title}
                lifetime={lifetime}
                carrier={carrier}
              />
            ),
          },
          { key: id },
        );
      }
      for (const view of release.views) {
        const binding = {
          installationId: installation.id,
          digest: release.digest,
          viewId: view.id,
        };
        const title = `${release.name} · ${view.title}`;
        switch (view.type) {
          case "schedules": {
            const reads = () => createScheduleViewReads(client.plugins, binding);
            lifetime.contribute(WORKSPACE_VIEW, {
              id: `package:${installation.id}:${view.id}`,
              title,
              icon: "calendar-clock",
              dock: "workspace",
              component: () => (
                <SchedulePackageView
                  reads={reads}
                  templates={view.scheduleTemplates ?? []}
                  title={title}
                  lifetime={lifetime}
                  carrier={carrier}
                />
              ),
            });
            break;
          }
        }
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
  modules: ComponentModules,
  generation: object,
): void {
  scope.cleanup(
    packageOperations.configure({
      ...client.plugins,
      signal: scope.signal,
      track: (operation) => trackContributionIO(scope, operation),
    }),
  );
  const fetcher = async (_params?: unknown, signal?: AbortSignal) =>
    (
      await trackContributionIO(
        scope,
        client.plugins.list(signal ? AbortSignal.any([scope.signal, signal]) : scope.signal),
      )
    ).data;
  scope.contribute(DATA_PROVIDER, { key: PACKAGES_KEY, fetcher });
  scope.contribute(DATA_PROVIDER, {
    key: PROVIDERS_KEY,
    fetcher: async (_params?: unknown, signal?: AbortSignal) =>
      (
        await trackContributionIO(
          scope,
          client.plugins.listProviders(
            signal ? AbortSignal.any([scope.signal, signal]) : scope.signal,
          ),
        )
      ).data,
  });

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
    const reconcile = createPackageReconciler(
      contributions,
      client,
      carrier,
      modules,
      generation,
      scope,
    );
    // The generation owns the task so it can join its contribution scope on failure.
    scope.spawn(async (signal) => {
      const refresh = async () => {
        // Cache cancellation cannot retire the generation's presentation resources.
        const rows = await fetcher(undefined, signal);
        if (signal.aborted) return;
        await reconcile(rows);
        if (!signal.aborted) {
          queryClient.setQueryData([PACKAGES_KEY], rows);
          await queryClient.invalidateQueries({ queryKey: [PROVIDERS_KEY] });
        }
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
          const unexpected = reasons.filter((cause) => !isCancellationReason(signal, cause));
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

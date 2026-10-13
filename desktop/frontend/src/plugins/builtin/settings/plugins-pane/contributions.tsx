import { lazy, Suspense, type ReactNode } from "react";
import { ComponentModules } from "./adapters/componentModules";
import {
  InstalledComponent,
  InstalledFileComponent,
  InstalledToolResultComponent,
  InstalledSessionActionComponent,
  ComponentBoundary,
  ComponentLoading,
} from "./ui/InstalledComponent";
import { currentRuntimeEndpoint } from "@/plugins/builtin/runtime/public/endpoint";
import { asyncDisposeSymbol, isCancellationReason } from "dougong";
import type { FlameClient } from "@flame/runtime-contract/client";
import type { PluginInstallation } from "@flame/runtime-contract/wire";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import {
  DATA_PROVIDER,
  WORKSPACE_VIEW,
  TOOL_RESULT_RENDERER,
  SESSION_ACTION,
  WORKSPACE_FILE_RENDERER,
} from "@/plugins/sdk/kernelPoints";
import { RecordedFileComponent } from "./ui/RecordedFileComponent";
import { RecordedToolComponent } from "./ui/RecordedToolComponent";
import type { CapturedFile, CapturedSession, CapturedToolResult } from "@flame/plugin-sdk";
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

const packageThemePrefix = "package:";

function packageThemeId(installation: string, theme: string): string {
  return `${packageThemePrefix}${installation}:${theme}`;
}

function createPackageReconciler(
  scope: ContributionLifetime,
  client: FlameClient,
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

        if (entry.placement === "fileRenderer") {
          if (!entry.selector || entry.selector.type !== "fileExtension")
            throw new Error("File component requires an extension selector");
          const File = lazy(async () => {
            const loaded = await modules.component(
              moduleScope,
              client,
              currentRuntimeEndpoint(),
              generation,
              installation,
              entry,
            );
            return {
              default: ({ file, fallback }: { file: CapturedFile; fallback: ReactNode }) => (
                <InstalledFileComponent
                  entry={loaded.component}
                  resources={loaded.signal}
                  runtime={client}
                  binding={binding}
                  lifetime={lifetime}
                  file={file}
                  fallback={fallback}
                />
              ),
            };
          });
          const id = `package:${installation.id}:${entry.id}`;
          lifetime.contribute(
            WORKSPACE_FILE_RENDERER,
            {
              id,
              title: `${release.name} · ${entry.title}`,
              extension: entry.selector.name,
              component: (props) => (
                <ComponentBoundary fallback={props.fallback}>
                  <Suspense fallback={props.fallback}>
                    <RecordedFileComponent {...props} entry={File} />
                  </Suspense>
                </ComponentBoundary>
              ),
            },
            { key: id },
          );
          continue;
        }
        if (entry.placement === "toolResultRenderer") {
          if (!entry.selector || entry.selector.type === "fileExtension")
            throw new Error("Tool result component requires a result selector");
          const Result = lazy(async () => {
            const loaded = await modules.component(
              moduleScope,
              client,
              currentRuntimeEndpoint(),
              generation,
              installation,
              entry,
            );
            return {
              default: ({
                toolResult,
                fallback,
              }: {
                toolResult: CapturedToolResult;
                fallback: ReactNode;
              }) => (
                <InstalledToolResultComponent
                  entry={loaded.component}
                  resources={loaded.signal}
                  runtime={client}
                  binding={binding}
                  lifetime={lifetime}
                  toolResult={toolResult}
                  fallback={fallback}
                />
              ),
            };
          });
          const id = `package:${installation.id}:${entry.id}`;
          lifetime.contribute(
            TOOL_RESULT_RENDERER,
            {
              id,
              title: `${release.name} · ${entry.title}`,
              selector: { ...entry.selector },
              component: (props) => (
                <ComponentBoundary fallback={props.fallback}>
                  <Suspense fallback={props.fallback}>
                    <RecordedToolComponent {...props} entry={Result} />
                  </Suspense>
                </ComponentBoundary>
              ),
            },
            { key: id },
          );
          continue;
        }
        if (entry.placement === "sessionAction") {
          const Action = lazy(async () => {
            const loaded = await modules.component(
              moduleScope,
              client,
              currentRuntimeEndpoint(),
              generation,
              installation,
              entry,
            );
            return {
              default: ({ session }: { session: CapturedSession }) => (
                <InstalledSessionActionComponent
                  entry={loaded.component}
                  resources={loaded.signal}
                  runtime={client}
                  binding={binding}
                  lifetime={lifetime}
                  session={session}
                />
              ),
            };
          });
          const id = `package:${installation.id}:${entry.id}`;
          lifetime.contribute(
            SESSION_ACTION,
            {
              id,
              title: `${release.name} · ${entry.title}`,
              component: Action,
            },
            { key: id },
          );
          continue;
        }
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
    const reconcile = createPackageReconciler(contributions, client, modules, generation, scope);
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

import { type ThemeAccess } from "@/features/theme/public/theme";
import { lazy, Suspense, type ReactNode } from "react";
import { ComponentModules } from "@/features/plugins/adapters/componentModules";
import {
  InstalledComponent,
  InstalledFileComponent,
  InstalledToolResultComponent,
  InstalledSessionActionComponent,
  ComponentBoundary,
  componentFailureScope,
  ComponentLoading,
} from "@/features/plugins/ui/InstalledComponent";
import { asyncDisposeSymbol, isCancellationReason } from "dougong";
import { type FlameClient } from "@flame/runtime-client";
import { type PluginInstallation } from "@flame/runtime-client/wire";
import { type ContributionLifetime } from "@/extensions/definePlugin";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { WORKSPACE_VIEW, WORKSPACE_FILE_RENDERER } from "@/features/workspace/public/views";
import { TOOL_RESULT_RENDERER } from "@/features/chat/public/tools/types";
import { SESSION_ACTION } from "@/features/navigation/public/sessionActions";
import { SETTINGS_PANE } from "@/features/settings/public/panes";
import { ComponentCompositions } from "@/features/plugins/ui/ComponentCompositions";
import { RecordedFileComponent } from "@/features/plugins/ui/RecordedFileComponent";
import { RecordedToolComponent } from "@/features/plugins/ui/RecordedToolComponent";
import {
  type CapturedFile,
  type CapturedSession,
  type CapturedToolResult,
  type ComponentContext,
} from "@flame/component-sdk";
import { queryClient } from "@/lib/queryClient";
import { failureMessage } from "@/lib/diagnostics";
import { contributePaletteTheme } from "@/features/theme/public/appearance";
import { retainThemeSelection } from "@/features/theme/public/appearance";
import { trackContributionIO } from "@/extensions/contributionIO";
import { reportPluginError } from "@/extensions/errors";
import { createLocaleReconciler } from "@/features/i18n/public/locales";
import { type RuntimeConnectionGeneration } from "@/features/connection/public/services";
import {
  type InstalledPackageState,
  PACKAGES_KEY,
  PROVIDERS_KEY,
} from "@/features/plugins/adapters/packages";
// A package-scoped id namespaces a contribution or theme by its installation so
// two packages cannot collide on one extension-point or selection id.
const packageIdPrefix = "package:";

function packageScopedId(installationId: string, localId: string): string {
  return `${packageIdPrefix}${installationId}:${localId}`;
}

function createPackageReconciler(
  scope: ContributionLifetime,
  client: FlameClient,
  modules: ComponentModules,
  generation: RuntimeConnectionGeneration,
  moduleScope: ContributionLifetime,
  endpoint: string,
  themeAccess: ThemeAccess,
  saveFile: ComponentContext["saveFile"],
): (installations: PluginInstallation[]) => Promise<void> {
  const compositions = new ComponentCompositions(
    client,
    modules,
    endpoint,
    generation,
    moduleScope,
  );
  const resources = new Map<string, { signature: string; lifetime: ContributionLifetime }>();
  const reconcileLocales = createLocaleReconciler(scope, (error) =>
    reportPluginError("flame.builtin.plugins-pane", "setup", error),
  );
  return async (installations: PluginInstallation[]) => {
    const admitted = installations.filter((item) => item.presentation === "admitted");
    await reconcileLocales(
      admitted.flatMap((installation) =>
        installation.selected.locales.map((locale) => {
          const request = {
            installationId: installation.id,
            digest: installation.selected.digest,
            tag: locale.tag,
          };
          return {
            key: JSON.stringify([request.installationId, request.digest, request.tag]),
            tag: locale.tag,
            title: locale.title,
            load: async (signal: AbortSignal) =>
              (await client.plugins.readLocale(request, signal)).messages,
          };
        }),
      ),
    );
    retainThemeSelection(
      themeAccess.preference,
      admitted.flatMap((item) =>
        item.selected.themes.map((theme) => packageScopedId(item.id, theme.id)),
      ),
      packageIdPrefix,
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
        if (entry.placement === "export") continue;
        const component = {
          installationId: installation.id,
          digest: release.digest,
          componentId: entry.id,
        };
        const compose = compositions.renderer(lifetime, [component]);
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
              endpoint,
              generation,
              installation,
              entry,
            );
            return {
              default: ({ file, fallback }: { file: CapturedFile; fallback: ReactNode }) => (
                <InstalledFileComponent
                  entry={loaded.component}
                  reload={loaded.reload}
                  component={component}
                  generation={generation}
                  compose={compose}
                  saveFile={saveFile}
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
          const id = packageScopedId(installation.id, entry.id);
          lifetime.contribute(
            WORKSPACE_FILE_RENDERER,
            {
              id,
              title: `${release.name} · ${entry.title}`,
              extension: entry.selector.name,
              component: (props) => (
                <ComponentBoundary
                  scope={componentFailureScope(component, generation)}
                  reload={() => modules.reload()}
                  fallback={props.fallback}
                >
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
              endpoint,
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
                  reload={loaded.reload}
                  component={component}
                  generation={generation}
                  compose={compose}
                  saveFile={saveFile}
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
          const id = packageScopedId(installation.id, entry.id);
          lifetime.contribute(
            TOOL_RESULT_RENDERER,
            {
              id,
              title: `${release.name} · ${entry.title}`,
              selector: { ...entry.selector },
              component: (props) => (
                <ComponentBoundary
                  scope={componentFailureScope(component, generation)}
                  reload={() => modules.reload()}
                  fallback={props.fallback}
                >
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
              endpoint,
              generation,
              installation,
              entry,
            );
            return {
              default: ({ session }: { session: CapturedSession }) => (
                <InstalledSessionActionComponent
                  entry={loaded.component}
                  reload={loaded.reload}
                  component={component}
                  generation={generation}
                  compose={compose}
                  saveFile={saveFile}
                  resources={loaded.signal}
                  runtime={client}
                  binding={binding}
                  lifetime={lifetime}
                  session={session}
                />
              ),
            };
          });
          const id = packageScopedId(installation.id, entry.id);
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
            endpoint,
            generation,
            installation,
            entry,
          );
          return {
            default: () => (
              <InstalledComponent
                entry={loaded.component}
                reload={loaded.reload}
                component={component}
                generation={generation}
                compose={compose}
                saveFile={saveFile}
                scope={entry.scope}
                resources={loaded.signal}
                runtime={client}
                binding={binding}
                lifetime={lifetime}
              />
            ),
          };
        });
        const id = packageScopedId(installation.id, entry.id);
        const PlacedComponent = () => (
          <ComponentBoundary
            scope={componentFailureScope(component, generation)}
            reload={() => modules.reload()}
          >
            <Suspense fallback={<ComponentLoading />}>
              <View />
            </Suspense>
          </ComponentBoundary>
        );
        if (entry.placement === "settingsPane") {
          lifetime.contribute(
            SETTINGS_PANE,
            {
              id,
              label: entry.title,
              material: entry.material,
              intent: entry.settingsIntent,
              group: "extensions",
              component: PlacedComponent,
            },
            { key: id },
          );
          continue;
        }
        lifetime.contribute(
          WORKSPACE_VIEW,
          {
            id,
            title: `${release.name} · ${entry.title}`,
            material: entry.material,
            dock: entry.scope === "session" ? "session" : undefined,
            component: PlacedComponent,
          },
          { key: id },
        );
      }
      for (const theme of release.themes) {
        const { background, foreground, accent, muted, border } = theme.colors;
        contributePaletteTheme(themeAccess.preference, lifetime, {
          id: packageScopedId(installation.id, theme.id),
          label: `${release.name} · ${theme.title}`,
          scheme: theme.scheme,
          palette: { background, foreground, accent, muted, border },
        });
      }
    }
  };
}

export function registerPackageContributions(
  packages: InstalledPackageState,
  scope: ContributionLifetime,
  client: FlameClient,
  modules: ComponentModules,
  generation: RuntimeConnectionGeneration,
  endpoint: string,
  themeAccess: ThemeAccess,
  saveFile: ComponentContext["saveFile"],
): void {
  scope.cleanup(
    packages.publishOperations({
      ...client.plugins,
      signal: scope.signal,
      track: (operation) => trackContributionIO(scope, operation),
    }),
  );
  const fetcher = async (_params?: unknown, signal?: AbortSignal) =>
    (
      await trackContributionIO(scope, () =>
        client.plugins.list(signal ? AbortSignal.any([scope.signal, signal]) : scope.signal),
      )
    ).data;
  scope.contribute(DATA_PROVIDER, { key: PACKAGES_KEY, fetcher });
  scope.contribute(DATA_PROVIDER, {
    key: PROVIDERS_KEY,
    fetcher: async (_params?: unknown, signal?: AbortSignal) =>
      (
        await trackContributionIO(scope, () =>
          client.plugins.listProviders(
            signal ? AbortSignal.any([scope.signal, signal]) : scope.signal,
          ),
        )
      ).data,
  });

  let failure: { reason: string; retry(): void } | null = null;
  const withdrawFailure = () => {
    if (failure && packages.failure() === failure) packages.setFailure(null);
    failure = null;
  };
  scope.cleanup(withdrawFailure);
  const realize = () => {
    withdrawFailure();
    const contributions = scope.lifetime("package-realization");
    const reconcile = createPackageReconciler(
      contributions,
      client,
      modules,
      generation,
      scope,
      endpoint,
      themeAccess,
      saveFile,
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
        packages.setFailure(failure);
      }
    });
  };
  realize();
}

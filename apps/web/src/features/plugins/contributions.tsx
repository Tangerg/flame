import { workflowAuthors } from "./adapters/workflowAuthors";
import { SharedEffects } from "./ui/SharedEffects";
import type { WorkspaceAccess } from "@/features/workspace/public/workspace";
import { type ThemeAccess } from "@/features/theme/public/theme";
import { ComponentModules } from "@/features/plugins/adapters/componentModules";
import { componentFailureScope } from "@/features/plugins/ui/InstalledComponent";
import { asyncDisposeSymbol, isCancellationReason } from "dougong";
import { onAbort } from "dougong/advanced";
import { type FlameClient } from "@flame/runtime-client";
import { type PluginInstallation } from "@flame/runtime-client/wire";
import { type ContributionScope } from "@/extensions/definePlugin";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { ComponentCompositions } from "@/features/plugins/ui/ComponentCompositions";
import { type ComponentContext } from "@flame/component-sdk";
import { queryClient } from "@/lib/queryClient";
import { failureMessage } from "@/lib/diagnostics";
import { retainThemeSelection } from "@/features/theme/public/appearance";
import { trackContributionIO } from "@/extensions/contributionIO";
import { reportPluginError } from "@/extensions/errors";
import { contributeLayout } from "@/extensions/contributeHelpers";
import { createLocaleReconciler } from "@/features/i18n/public/locales";
import { type RuntimeConnectionGeneration } from "@/features/connection/public/services";
import {
  type InstalledPackageState,
  type PackageRealizationFailure,
  INSTALLED_PLUGIN_LOADER,
  PACKAGE_ID_PREFIX,
  PACKAGES_KEY,
  packageScopedId,
  PROVIDERS_KEY,
} from "@/features/plugins/adapters/packages";
import {
  contributeComponent,
  contributeThemes,
  localeSources,
  operationBinding,
} from "./contributionKinds";

function createPackageReconciler(
  scope: ContributionScope,
  client: FlameClient,
  modules: ComponentModules,
  generation: RuntimeConnectionGeneration,
  moduleScope: ContributionScope,
  endpoint: string,
  workspace: WorkspaceAccess,
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
  const resources = new Map<string, { signature: string; lifetime: ContributionScope }>();
  const reconcileLocales = createLocaleReconciler(scope, (error) =>
    reportPluginError(INSTALLED_PLUGIN_LOADER, "setup", error),
  );
  const contributeInstallation = (
    lifetime: ContributionScope,
    installation: PluginInstallation,
  ) => {
    const release = installation.selected;
    for (const entry of release.components) {
      if (entry.placement === "export") continue;
      const component = {
        installationId: installation.id,
        digest: release.digest,
        componentId: entry.id,
      };
      const compose = compositions.renderer(lifetime, [component]);
      const binding = operationBinding(installation, entry);
      contributeComponent(
        {
          lifetime,
          installation,
          entry,
          id: packageScopedId(installation.id, entry.id),
          failureScope: componentFailureScope(component, generation),
          modules,
          load: async () => {
            const loaded = await modules.component(
              moduleScope,
              client,
              endpoint,
              generation,
              installation,
              entry,
            );
            return {
              entry: loaded.component,
              reload: loaded.reload,
              component,
              generation,
              compose,
              saveFile,
              resources: loaded.signal,
              runtime: client,
              binding,
              lifetime,
            };
          },
        },
        workspace,
      );
    }
    contributeThemes(themeAccess, lifetime, installation);
  };
  return async (installations: PluginInstallation[]) => {
    const admitted = installations.filter((item) => item.presentation === "admitted");
    // A withheld but enabled package is still the user's language source; only
    // disabling or removing it retires the dictionary kept from the last launch.
    const offered = new Set(
      installations
        .filter((item) => item.enabled)
        .flatMap((item) => item.selected.locales.map((locale) => locale.tag)),
    );
    await reconcileLocales(localeSources(client, admitted), offered);
    retainThemeSelection(
      themeAccess.preference,
      admitted.flatMap((item) =>
        item.selected.themes.map((theme) => packageScopedId(item.id, theme.id)),
      ),
      PACKAGE_ID_PREFIX,
    );
    const ids = new Set(admitted.map((item) => item.id));
    for (const [id, resource] of resources) {
      if (!ids.has(id)) {
        resources.delete(id);
        await resource.lifetime[asyncDisposeSymbol]();
      }
    }
    for (const installation of admitted) {
      if (scope.lifetime.signal.aborted) return;
      const signature = installation.selected.digest;
      const previous = resources.get(installation.id);
      if (previous?.signature === signature) continue;
      if (previous) {
        resources.delete(installation.id);
        await previous.lifetime[asyncDisposeSymbol]();
      }
      if (scope.lifetime.signal.aborted) return;
      const lifetime = scope.child(installation.id);
      resources.set(installation.id, { signature, lifetime });
      contributeInstallation(lifetime, installation);
    }
  };
}

export function registerPackageContributions(
  packages: InstalledPackageState,
  scope: ContributionScope,
  client: FlameClient,
  modules: ComponentModules,
  generation: RuntimeConnectionGeneration,
  endpoint: string,
  workspace: WorkspaceAccess,
  themeAccess: ThemeAccess,
  saveFile: ComponentContext["saveFile"],
): void {
  contributeLayout(scope, "app.overlay", {
    id: "installed-shared-effects",
    component: SharedEffects,
  });
  scope.lifetime.defer(
    onAbort(
      scope.lifetime.signal,
      packages.publishOperations({
        ...client.plugins,
        signal: scope.lifetime.signal,
        track: (operation) => trackContributionIO(scope, operation),
      }),
    ),
  );
  const fetcher = async (_params?: unknown, signal?: AbortSignal) =>
    (await scope.lifetime.run((owned) => client.plugins.list(owned), signal)).value.data;
  scope.add(DATA_PROVIDER, { key: PACKAGES_KEY, fetcher });
  const fetchProviders = async (_params?: unknown, signal?: AbortSignal) =>
    (await scope.lifetime.run((owned) => client.plugins.listProviders(owned), signal)).value.data;
  scope.add(DATA_PROVIDER, { key: PROVIDERS_KEY, fetcher: fetchProviders });

  let failure: PackageRealizationFailure | null = null;
  const withdrawFailure = () => {
    if (failure && packages.failure() === failure) packages.setFailure(null);
    failure = null;
  };
  scope.lifetime.defer(withdrawFailure);
  const realize = () => {
    withdrawFailure();
    const contributions = scope.child("package-realization");
    contributions.lifetime.defer(() => packages.workflows.publish([], []));
    const reconcile = createPackageReconciler(
      contributions,
      client,
      modules,
      generation,
      scope,
      endpoint,
      workspace,
      themeAccess,
      saveFile,
    );
    // The generation owns the task so it can join its contribution scope on failure.
    scope.lifetime.spawn(async (signal) => {
      const refresh = async () => {
        // Cache cancellation cannot retire the generation's presentation resources.
        const rows = await fetcher(undefined, signal);
        if (signal.aborted) return;
        await reconcile(rows);
        if (!signal.aborted) {
          queryClient.setQueryData([PACKAGES_KEY], rows);
          try {
            const choices = await fetchProviders(undefined, signal);
            if (signal.aborted) return;
            packages.workflows.publish(workflowAuthors(rows), choices);
            queryClient.setQueryData([PROVIDERS_KEY], choices);
          } catch {
            if (!signal.aborted) {
              packages.workflows.publish(workflowAuthors(rows), []);
              await queryClient.invalidateQueries({ queryKey: [PROVIDERS_KEY] });
            }
          }
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
        const closeEvents = contributions.lifetime.defer(() => events.return?.());
        await refresh();
        while (!(await events.next()).done) await refresh();
        await closeEvents[asyncDisposeSymbol]();
      } catch (error) {
        const reasons = [error];
        let released = false;
        try {
          await contributions[asyncDisposeSymbol]();
          released = true;
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
        const failed: PackageRealizationFailure = {
          reason: reasons.map(failureMessage).join("; "),
          retry: released
            ? () => {
                if (failure === failed && !scope.lifetime.signal.aborted) realize();
              }
            : undefined,
        };
        failure = failed;
        packages.setFailure(failure);
      }
    });
  };
  realize();
}

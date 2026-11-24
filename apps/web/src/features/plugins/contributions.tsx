import { workflowAuthors } from "./adapters/workflowAuthors";
import { InstalledMessageAction } from "@/features/plugins/ui/InstalledMessageAction";
import { SHARED_CLIENT_EFFECT, SharedEffects } from "./ui/SharedEffects";
import { tupleKey } from "@/lib/tupleKey";
import { WorkspaceChangesAction } from "./ui/WorkspaceChangesAction";
import { TOOL_VIEW_OPENER } from "@/features/chat/public/tools/types";
import type { WorkspaceAccess } from "@/features/workspace/public/workspace";
import { openWorkspaceViewAtPath } from "@/features/workspace/public/navigation";
import { presentationTitle } from "@/lib/i18n/presentationTitle";
import { activeLocale } from "@/lib/i18n/index";
import { type ThemeAccess } from "@/features/theme/public/theme";
import { lazy, Suspense, type ReactNode } from "react";
import { ComponentModules } from "@/features/plugins/adapters/componentModules";
import {
  InstalledComponent,
  RenderingComponent,
  InstalledFileComponent,
  InstalledToolResultComponent,
  InstalledSessionActionComponent,
  ComponentBoundary,
  componentFailureScope,
  ComponentLoading,
  type InstalledComponentProps,
} from "@/features/plugins/ui/InstalledComponent";
import { asyncDisposeSymbol, isCancellationReason } from "dougong";
import { onAbort } from "dougong/advanced";
import { type FlameClient } from "@flame/runtime-client";
import { type PluginInstallation } from "@flame/runtime-client/wire";
import { type ContributionScope } from "@/extensions/definePlugin";
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

function operationBinding(
  installation: PluginInstallation,
  entry: PluginInstallation["selected"]["components"][number],
) {
  const release = installation.selected;
  const contribution = release.contributions.find((value) => value.id === entry.contribution);
  if (entry.contribution && !contribution)
    throw new Error("The component's declared callable contribution is unavailable.");
  return contribution
    ? {
        installationId: installation.id,
        digest: release.digest,
        contribution: contribution.id,
        contract: contribution.contract,
      }
    : undefined;
}

type ComponentEntry = PluginInstallation["selected"]["components"][number];

// One installed component and the scope that owns everything it contributes.
interface PackageComponent {
  lifetime: ContributionScope;
  installation: PluginInstallation;
  entry: ComponentEntry;
  id: string;
  qualifiedTitle: string;
  failureScope: string;
  modules: ComponentModules;
  load: () => Promise<InstalledComponentProps>;
}

function contributeComponent(target: PackageComponent, workspace: WorkspaceAccess): void {
  switch (target.entry.placement) {
    case "export":
      return;
    case "fileRenderer":
      return contributeFileRenderer(target);
    case "toolResultRenderer":
      return contributeToolResultRenderer(target);
    case "sessionAction":
      return contributeSessionAction(target);
    default:
      return contributePlacedComponent(target, workspace);
  }
}

function contributeFileRenderer({
  lifetime,
  entry,
  id,
  qualifiedTitle,
  failureScope,
  modules,
  load,
}: PackageComponent) {
  if (!entry.selector || entry.selector.type !== "fileExtension")
    throw new Error("File component requires an extension selector");
  const File = lazy(async () => {
    const rendering = await load();
    return {
      default: ({ file, fallback }: { file: CapturedFile; fallback: ReactNode }) => (
        <InstalledFileComponent {...rendering} file={file} fallback={fallback} />
      ),
    };
  });
  lifetime.add(
    WORKSPACE_FILE_RENDERER,
    {
      id,
      title: qualifiedTitle,
      extension: entry.selector.name,
      component: (props) => (
        <ComponentBoundary
          scope={failureScope}
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
}

function contributeToolResultRenderer({
  lifetime,
  entry,
  id,
  qualifiedTitle,
  failureScope,
  modules,
  load,
}: PackageComponent) {
  if (!entry.selector || entry.selector.type === "fileExtension")
    throw new Error("Tool result component requires a result selector");
  const Result = lazy(async () => {
    const rendering = await load();
    return {
      default: ({
        toolResult,
        fallback,
      }: {
        toolResult: CapturedToolResult;
        fallback: ReactNode;
      }) => (
        <InstalledToolResultComponent {...rendering} toolResult={toolResult} fallback={fallback} />
      ),
    };
  });
  lifetime.add(
    TOOL_RESULT_RENDERER,
    {
      id,
      title: qualifiedTitle,
      selector:
        entry.selector.type === "resultShape"
          ? { type: "resultShape", shape: entry.selector.shape }
          : { type: "toolName", name: entry.selector.name },
      component: (props) => (
        <ComponentBoundary
          scope={failureScope}
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
}

function contributeSessionAction({ lifetime, entry, id, qualifiedTitle, load }: PackageComponent) {
  const Action = lazy(async () => {
    const rendering = await load();
    return {
      default: ({ session }: { session: CapturedSession }) => (
        <InstalledSessionActionComponent {...rendering} session={session} />
      ),
    };
  });
  lifetime.add(
    SESSION_ACTION,
    {
      id,
      title: qualifiedTitle,
      localizedTitles: entry.localizedTitles,
      component: Action,
    },
    { key: id },
  );
}

// Effects and toolbar actions have no surface of their own to show loading or recovery in.
const SURFACELESS_PLACEMENTS = new Set<ComponentEntry["placement"]>([
  "clientEffect",
  "sharedClientEffect",
  "messageAction",
  "workspaceChangesAction",
]);

function contributePlacedComponent(
  {
    lifetime,
    installation,
    entry,
    id,
    qualifiedTitle,
    failureScope,
    modules,
    load,
  }: PackageComponent,
  workspace: WorkspaceAccess,
) {
  const { placement } = entry;
  const View = lazy(async () => {
    const rendering = { placement, ...(await load()) };
    switch (placement) {
      case "messageAction":
        return { default: () => <InstalledMessageAction {...rendering} /> };
      case "workspaceChangesAction":
        return {
          default: () => (
            <WorkspaceChangesAction
              {...rendering}
              targetView={packageScopedId(installation.id, entry.targetView!)}
            />
          ),
        };
      case "clientEffect":
      case "sharedClientEffect":
        return { default: () => <RenderingComponent {...rendering} /> };
      default:
        return { default: () => <InstalledComponent {...rendering} scope={entry.scope} /> };
    }
  });
  const surfaceless = SURFACELESS_PLACEMENTS.has(placement);
  const PlacedComponent = () => (
    <ComponentBoundary
      scope={failureScope}
      reload={() => modules.reload()}
      report={surfaceless ? modules.report : undefined}
    >
      <Suspense fallback={surfaceless ? null : <ComponentLoading />}>
        <View />
      </Suspense>
    </ComponentBoundary>
  );
  switch (placement) {
    case "messageAction":
      contributeLayout(lifetime, "message.actions", { id, order: 15, component: PlacedComponent });
      return;
    case "workspaceChangesAction":
      contributeLayout(lifetime, "chat.header.meta", { id, component: PlacedComponent });
      return;
    case "sharedClientEffect":
      lifetime.add(
        SHARED_CLIENT_EFFECT,
        {
          id,
          group: tupleKey(installation.selected.name, entry.id),
          title: entry.title,
          component: PlacedComponent,
        },
        { key: id },
      );
      return;
    case "clientEffect":
      contributeLayout(lifetime, "app.overlay", { id, component: PlacedComponent });
      return;
    case "settingsPane":
      lifetime.add(
        SETTINGS_PANE,
        {
          id,
          label: entry.title,
          localizedTitles: entry.localizedTitles,
          keywords: [entry.title, entry.id],
          material: entry.material,
          intent: entry.settingsIntent,
          group: "extensions",
          component: PlacedComponent,
        },
        { key: id },
      );
      return;
  }
  if (entry.toolCategory) {
    lifetime.add(
      TOOL_VIEW_OPENER,
      {
        id,
        predicate: (tool) => tool.category === entry.toolCategory,
        label: () => presentationTitle(entry.title, entry.localizedTitles, activeLocale()),
        open: (tool, origin) => {
          lifetime.lifetime.signal.throwIfAborted();
          openWorkspaceViewAtPath(workspace, id, tool.fnKind === "path" ? tool.fn : "", origin);
        },
      },
      { key: id },
    );
  }
  lifetime.add(
    WORKSPACE_VIEW,
    {
      id,
      title: qualifiedTitle,
      localizedTitles: entry.localizedTitles,
      material: entry.material,
      dock: entry.scope === "session" ? "session" : undefined,
      component: PlacedComponent,
    },
    { key: id },
  );
}

function contributeThemes(
  themeAccess: ThemeAccess,
  lifetime: ContributionScope,
  installation: PluginInstallation,
) {
  const release = installation.selected;
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

function localeSources(client: FlameClient, admitted: PluginInstallation[]) {
  return admitted.flatMap((installation) =>
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
  );
}

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
          qualifiedTitle: `${release.name} · ${entry.title}`,
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

import { InstalledMessageAction } from "@/features/plugins/ui/InstalledMessageAction";
import { SHARED_CLIENT_EFFECT } from "./ui/SharedEffects";
import { tupleKey } from "@/lib/tupleKey";
import { WorkspaceChangesAction } from "./ui/WorkspaceChangesAction";
import { TOOL_VIEW_OPENER, TOOL_RESULT_RENDERER } from "@/features/chat/public/tools/types";
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
  ComponentLoading,
  type InstalledComponentProps,
} from "@/features/plugins/ui/InstalledComponent";
import { type FlameClient } from "@flame/runtime-client";
import { type PluginInstallation } from "@flame/runtime-client/wire";
import { type ContributionScope } from "@/extensions/definePlugin";
import { WORKSPACE_VIEW, WORKSPACE_FILE_RENDERER } from "@/features/workspace/public/views";
import { SESSION_ACTION } from "@/features/navigation/public/sessionActions";
import { SETTINGS_PANE } from "@/features/settings/public/panes";
import { settingsGroupForIntent } from "@/features/settings/public/intent";
import { RecordedFileComponent } from "@/features/plugins/ui/RecordedFileComponent";
import { RecordedToolComponent } from "@/features/plugins/ui/RecordedToolComponent";
import {
  type CapturedFile,
  type CapturedSession,
  type CapturedToolResult,
} from "@flame/component-sdk";
import { contributePaletteTheme } from "@/features/theme/public/appearance";
import { contributeLayout } from "@/extensions/contributeHelpers";
import { packageScopedId } from "@/features/plugins/adapters/packages";

export function operationBinding(
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
  failureScope: string;
  modules: ComponentModules;
  load: () => Promise<InstalledComponentProps>;
}

export function contributeComponent(target: PackageComponent, workspace: WorkspaceAccess): void {
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
      title: { text: entry.title },
      localizedTitles: entry.localizedTitles,
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
      title: { text: entry.title },
      localizedTitles: entry.localizedTitles,
      inlineResult: entry.inlineResult,
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
          <Suspense fallback={props.fallback ?? <ComponentLoading />}>
            <RecordedToolComponent {...props} entry={Result} />
          </Suspense>
        </ComponentBoundary>
      ),
    },
    { key: id },
  );
}

function contributeSessionAction({ lifetime, entry, id, load }: PackageComponent) {
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
      title: entry.title,
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
  { lifetime, installation, entry, id, failureScope, modules, load }: PackageComponent,
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
          label: { text: entry.title },
          localizedTitles: entry.localizedTitles,
          keywords: [{ text: entry.title }, { text: entry.id }],
          material: entry.material,
          intent: entry.settingsIntent,
          group: settingsGroupForIntent(entry.settingsIntent),
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
        label: () => ({
          text: presentationTitle(entry.title, entry.localizedTitles, activeLocale()),
        }),
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
      title: { text: entry.title },
      localizedTitles: entry.localizedTitles,
      material: entry.material,
      dock: entry.scope === "session" ? "session" : undefined,
      component: PlacedComponent,
    },
    { key: id },
  );
}

export function contributeThemes(
  themeAccess: ThemeAccess,
  lifetime: ContributionScope,
  installation: PluginInstallation,
) {
  const release = installation.selected;
  for (const theme of release.themes) {
    const { background, foreground, accent, muted, border } = theme.colors;
    contributePaletteTheme(themeAccess.preference, lifetime, {
      id: packageScopedId(installation.id, theme.id),
      label: { text: `${release.name} · ${theme.title}` },
      scheme: theme.scheme,
      palette: { background, foreground, accent, muted, border },
    });
  }
}

export function localeSources(client: FlameClient, admitted: PluginInstallation[]) {
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

import { visualFilePage } from "./filePage";
import { type NotificationFeed } from "@/features/notifications/public/notifications";
import { type VisualClient } from "./client";
import { isCancelledError, type Query } from "@tanstack/react-query";
import { queryClient } from "@/lib/queryClient";
import webPackageContent from "../../package.json?raw";
import { type NotificationLevel } from "@/features/notifications/public/types";
import { InstalledComponent } from "@/features/plugins/ui/InstalledComponent";
import { WorkspaceChangesAction } from "@/features/plugins/ui/WorkspaceChangesAction";
import { Schedules } from "../../../../plugins/schedules/component/component";
import { Hooks } from "../../../../plugins/hooks/component/component";
import { type ComponentDefinition, type PluginComponent } from "@flame/component-sdk";
import brandDeclaration from "../../../../plugins/brand/package/io.github.tangerg.flame/declaration.json";
import mcpDeclaration from "../../../../plugins/mcp/package/io.github.tangerg.flame/declaration.json";
import providersDeclaration from "../../../../plugins/providers/package/io.github.tangerg.flame/declaration.json";
import hooksDeclaration from "../../../../plugins/hooks/package/io.github.tangerg.flame/declaration.json";
import diagnosticsDeclaration from "../../../../plugins/diagnostics/package/io.github.tangerg.flame/declaration.json";
import galleryDeclaration from "../../../../plugins/icon-gallery/package/io.github.tangerg.flame/declaration.json";
import trajectoryDeclaration from "../../../../plugins/trajectory/package/io.github.tangerg.flame/declaration.json";
import diffDeclaration from "../../../../plugins/diff/package/io.github.tangerg.flame/declaration.json";
import usageDeclaration from "../../../../plugins/usage/package/io.github.tangerg.flame/declaration.json";
import usageReportsDeclaration from "../../../../plugins/usagereports/package/io.github.tangerg.flame/declaration.json";
import memoryDeclaration from "../../../../plugins/memory/package/io.github.tangerg.flame/declaration.json";
import schedulesDeclaration from "../../../../plugins/schedules/package/io.github.tangerg.flame/declaration.json";
import skillsDeclaration from "../../../../plugins/skills/package/io.github.tangerg.flame/declaration.json";
import { visualComponentProps } from "./installedComponentProps";
import { visualFileRenderers } from "./visualFileRenderers";
import { Gallery } from "../../../../plugins/icon-gallery/component/component";
import { Diagnostics } from "../../../../plugins/diagnostics/component/component";
import { type Diff, type GetDiffRequest } from "@flame/runtime-client/wire";
import { Usage } from "../../../../plugins/usage/component/component";
import { Memory } from "../../../../plugins/memory/component/component";
import { createElement } from "react";
import { createBrowserHost } from "@/platform/browserHost";
import { WORKSPACE_DOCK_CATALOG } from "@/features/workspace/public/navigation";
import { splitFilePath } from "@/lib/path";
import appearanceSettings from "@/features/settings/appearance";
import { createProviderCatalogPlugin } from "@/features/providers/index";
import approvalsSettings from "@/features/approvals/index";
import connectionSettings from "@/features/connection/settings/index";
import { createInstalledPluginLoader, pluginManagementPane } from "@/features/plugins/index";
import {
  WORKSPACE_FILES_CHANGED_KEY,
  WORKSPACE_LIST_FILES_KEY,
  WORKSPACE_READ_FILE_KEY,
  type WorkspaceFileChange,
  type WorkspaceFileContent,
  type WorkspaceFileEntry,
  type WorkspaceListFilesQuery,
  type WorkspaceReadFileQuery,
} from "@/features/workspace/application/workspaceQueries";
import {
  BRANCH_REVIEW_DIFF,
  reviewDiff,
  branchReviewDiff,
  scaledReview,
  evidenceDiff,
  budgetDiff,
  largeFileDiff,
  ACTIVE_DIFF_FILE,
} from "./diffSnapshot";
import { VISUAL_CAPABILITIES, VISUAL_WORKSPACE_PATH } from "./agentFixtureFacts";
import {
  VISUAL_DIAGNOSTICS,
  VISUAL_MEMORY_BINDING,
  VISUAL_SCHEDULES_BINDING,
  VISUAL_USAGE_REPORTS_BINDING,
} from "./componentSnapshots";
import { useTelemetryStore } from "@/lib/observability/stores";
import { Skills } from "../../../../plugins/skills/component/component";
import { fileView } from "@/features/workspace/views";
import { createWorkspaceEventsPlugin } from "@/features/workspace/events";
import {
  WORKSPACE_MUTATION_LIFECYCLE,
  type WorkspaceMutationLifecycle,
} from "@/features/workspace/public/services";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { contributeLayout } from "@/extensions/contributeHelpers";
import { SETTINGS_PANE } from "@/features/settings/public/panes";
import { SHORTCUT } from "@/features/command/public/extensionPoints";
import { WORKSPACE_VIEW } from "@/features/workspace/public/views";
import { definePlugin } from "@/extensions/definePlugin";
import { type AnyPlugin } from "dougong";
import { RpcError, type FlameClient } from "@flame/runtime-client";
import { WorkspaceFileFocus } from "@/features/workspace/adapters/contextDockStore";
import {
  RUNTIME_AGENT_SESSION_SNAPSHOTS,
  CALLOUT_MARKDOWN,
  LONG_RESPONSE_MARKDOWN,
  READING_BLOCKS_MARKDOWN,
  VISUAL_SESSION_ID,
  type VisualAgentState,
} from "./agentSessionSnapshots";
import { installVisualAgentFixture } from "./installVisualAgentFixture";
import { loadPluginsForTest } from "@/test/kernel";
import {
  VISUAL_DOCK_WIDTH_RATIO,
  VISUAL_REVIEW_DOCK_WIDTH_RATIO,
  type VisualWorkspaceState,
  type VisualSettingsPane,
  type VisualWorkspaceTheme,
  DOCK_VIEW_BY_STATE,
  VISUAL_DIRECTORY_PATH,
  VISUAL_DIRECTORY_CHANGED_EVENT,
  type VisualDirectoryState,
} from "./workspaceFixtureStates";
const Brand = Object.values(
  import.meta.glob<{ Brand: PluginComponent }>(
    "../../../../plugins/brand/package/io.github.tangerg.flame/components/component.js",
    { eager: true },
  ),
)[0]!.Brand;
const Providers = Object.values(
  import.meta.glob<{ Providers: ComponentDefinition }>(
    "../../../../plugins/providers/package/io.github.tangerg.flame/components/component.js",
    { eager: true },
  ),
)[0]!.Providers;
const MCPServers = Object.values(
  import.meta.glob<{ MCPServers: ComponentDefinition }>(
    "../../../../plugins/mcp/package/io.github.tangerg.flame/components/component.js",
    { eager: true },
  ),
)[0]!.MCPServers;
const { WorktreeChanges, WorkspaceDiff } = Object.values(
  import.meta.glob<{ WorktreeChanges: PluginComponent; WorkspaceDiff: PluginComponent }>(
    "../../../../plugins/diff/package/io.github.tangerg.flame/components/component.js",
    { eager: true },
  ),
)[0]!;
const Trajectory = Object.values(
  import.meta.glob<{ Trajectory: PluginComponent }>(
    "../../../../plugins/trajectory/package/io.github.tangerg.flame/components/component.js",
    { eager: true },
  ),
)[0]!.Trajectory;
const UsageReports = Object.values(
  import.meta.glob<{ UsageReports: PluginComponent }>(
    "../../../../plugins/usagereports/package/io.github.tangerg.flame/components/component.js",
    { eager: true },
  ),
)[0]!.UsageReports;
const RESIZER_SOURCE: WorkspaceFileContent = {
  startLine: 1,
  totalLines: 8,
  endLine: 8,
  content: [
    "const onKeyDown = useCallback((event: React.KeyboardEvent<HTMLDivElement>) => {",
    '  if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;',
    "  const row = railRef.current?.parentElement;",
    "  if (!row) return;",
    "  const currentWidth = readDockWidth(row);",
    "  const nextWidth = clampDockWidth(currentWidth + delta, row.clientWidth);",
    "  row.style.setProperty(DOCK_WIDTH_PROPERTY, `${nextWidth}px`);",
    "}, [setWidth]);",
  ].join("\n"),
};

const README_CONTENT = [
  "# Workspace notes",
  "",
  "Keep the application boundary clear. **Runtime owns execution**; the interface presents the recorded work.",
  "",
  "## Review checklist",
  "",
  "- Inspect the riskiest change first.",
  "- Verify the public contract and its callers.",
  "- Keep every fact with one owner.",
  "",
  "> Preserve the original evidence when investigating a failure.",
  "",
  "### Verification",
  "",
  "Run `npm run check` before reviewing the visual changes.",
  "",
  "```ts",
  'const outcome = "recorded";',
  "```",
  "",
  "| Surface | Check |",
  "| --- | --- |",
  "| Memory | Review and edit |",
  "| Schedules | Create and inspect |",
].join("\n");

const README_SOURCE: WorkspaceFileContent = {
  content: README_CONTENT,
  startLine: 1,
  totalLines: README_CONTENT.split("\n").length,
  endLine: README_CONTENT.split("\n").length,
};

const WEB_PACKAGE_SOURCE: WorkspaceFileContent = {
  content: webPackageContent,
  startLine: 1,
  totalLines: webPackageContent.split("\n").length,
  endLine: webPackageContent.split("\n").length,
};

const UNSUPPORTED_IMAGE: WorkspaceFileEntry = {
  path: "review-image.png",
  name: "review-image.png",
  type: "file",
  sizeBytes: 8_192,
};

const READING_GUIDE_SOURCE: WorkspaceFileContent = {
  content: LONG_RESPONSE_MARKDOWN,
  startLine: 1,
  totalLines: LONG_RESPONSE_MARKDOWN.split("\n").length,
  endLine: LONG_RESPONSE_MARKDOWN.split("\n").length,
};

const READING_BLOCKS_SOURCE: WorkspaceFileContent = {
  content: READING_BLOCKS_MARKDOWN,
  startLine: 1,
  totalLines: READING_BLOCKS_MARKDOWN.split("\n").length,
  endLine: READING_BLOCKS_MARKDOWN.split("\n").length,
};

const READING_CALLOUTS_SOURCE: WorkspaceFileContent = {
  content: CALLOUT_MARKDOWN,
  startLine: 1,
  totalLines: CALLOUT_MARKDOWN.split("\n").length,
  endLine: CALLOUT_MARKDOWN.split("\n").length,
};

function publishedComponent(
  declaration: {
    readonly components: readonly {
      readonly id: string;
      readonly title: string;
      readonly localizedTitles?: Readonly<Record<string, string>>;
    }[];
  },
  id: string,
) {
  const entry = declaration.components.find((component) => component.id === id);
  if (!entry) throw new Error(`published declaration has no component "${id}"`);
  return { id: entry.id, title: entry.title, localizedTitles: entry.localizedTitles };
}

function pending<T>(signal?: AbortSignal): Promise<T> {
  signal?.throwIfAborted();
  return new Promise<T>((_, reject) => {
    signal?.addEventListener("abort", () => reject(signal.reason), { once: true });
  });
}

function workspaceDataPlugin(
  readDiff: (signal?: AbortSignal) => Promise<Diff>,
  runtimeClient: () => FlameClient,
  state: VisualWorkspaceState,
  mutationLifecycle: WorkspaceMutationLifecycle,
  directoryState?: VisualDirectoryState,
): AnyPlugin {
  return definePlugin({
    name: "flame.visual.workspace-data",
    provides: { mutationLifecycle: WORKSPACE_MUTATION_LIFECYCLE },
    setup(ctx) {
      const brandLifetime = ctx.contributions.child("visual-brand");
      const brandSources = visualComponentProps(runtimeClient(), brandLifetime, "brand");
      const saveBrandFile = createBrowserHost().saveFile;
      const brandPane = publishedComponent(brandDeclaration, "brand");
      brandLifetime.add(SETTINGS_PANE, {
        id: "brand",
        label: brandPane.title,
        localizedTitles: brandPane.localizedTitles,
        keywords: [brandPane.title, brandPane.id],
        group: "extensions",
        component: () =>
          createElement(InstalledComponent, {
            ...brandSources,
            scope: "client",
            entry: Brand,
            saveFile: saveBrandFile,
          }),
      });
      const providersLifetime = ctx.contributions.child("visual-providers");
      const providersSources = visualComponentProps(
        runtimeClient(),
        providersLifetime,
        "providers",
      );
      const ProviderSettings = Providers.create({
        runtimeIdentity: providersSources.runtime,
        component: providersSources.component,
        signal: providersLifetime.lifetime.signal,
      });
      const providersPane = publishedComponent(providersDeclaration, "settings");
      providersLifetime.add(SETTINGS_PANE, {
        id: "providers",
        label: providersPane.title,
        localizedTitles: providersPane.localizedTitles,
        keywords: [providersPane.title, providersPane.id],
        material: "providers",
        intent: "model-providers",
        group: "extensions",
        component: () =>
          createElement(InstalledComponent, {
            ...providersSources,
            scope: "client",
            entry: ProviderSettings,
          }),
      });
      const mcpLifetime = ctx.contributions.child("visual-mcp");
      const mcpSources = visualComponentProps(runtimeClient(), mcpLifetime, "mcp");
      const MCPSettingsView = MCPServers.create({
        runtimeIdentity: mcpSources.runtime,
        component: mcpSources.component,
        signal: mcpLifetime.lifetime.signal,
      });
      const mcpPane = publishedComponent(mcpDeclaration, "settings");
      mcpLifetime.add(SETTINGS_PANE, {
        id: "mcp-servers",
        label: mcpPane.title,
        localizedTitles: mcpPane.localizedTitles,
        keywords: [mcpPane.title, mcpPane.id],
        intent: "mcp-servers",
        group: "extensions",
        component: () =>
          createElement(InstalledComponent, {
            ...mcpSources,
            scope: "client",
            entry: MCPSettingsView,
          }),
      });
      const hooksLifetime = ctx.contributions.child("visual-hooks");
      const hooksSources = visualComponentProps(runtimeClient(), hooksLifetime, "hooks");
      const hooksPane = publishedComponent(hooksDeclaration, "settings");
      hooksLifetime.add(SETTINGS_PANE, {
        id: "hooks",
        label: hooksPane.title,
        localizedTitles: hooksPane.localizedTitles,
        keywords: [hooksPane.title, hooksPane.id],
        group: "extensions",
        component: () =>
          createElement(InstalledComponent, { ...hooksSources, scope: "client", entry: Hooks }),
      });
      const diagnosticsLifetime = ctx.contributions.child("visual-diagnostics");
      const diagnosticsSources = visualComponentProps(
        runtimeClient(),
        diagnosticsLifetime,
        "diagnostics",
      );
      const diagnosticsView = publishedComponent(diagnosticsDeclaration, "diagnostics");
      diagnosticsLifetime.add(WORKSPACE_VIEW, {
        id: "package:visual:diagnostics",
        title: diagnosticsView.title,
        localizedTitles: diagnosticsView.localizedTitles,
        icon: "activity",
        dock: "workspace",
        material: "diagnostics",
        component: () =>
          createElement(InstalledComponent, {
            ...diagnosticsSources,
            scope: "installation",
            entry: Diagnostics,
          }),
      });
      const galleryLifetime = ctx.contributions.child("visual-icon-gallery");
      const gallerySources = visualComponentProps(runtimeClient(), galleryLifetime, "icon-gallery");
      const galleryView = publishedComponent(galleryDeclaration, "icon-gallery");
      galleryLifetime.add(WORKSPACE_VIEW, {
        id: "package:visual:icon-gallery",
        title: galleryView.title,
        localizedTitles: galleryView.localizedTitles,
        component: () =>
          createElement(InstalledComponent, {
            ...gallerySources,
            scope: "installation",
            entry: Gallery,
          }),
      });
      const lifetime = ctx.contributions.child("visual-trajectory");
      const trajectorySources = visualComponentProps(runtimeClient(), lifetime, "trajectory");
      const trajectoryView = publishedComponent(trajectoryDeclaration, "trajectory");
      lifetime.add(WORKSPACE_VIEW, {
        id: "package:visual:trajectory",
        title: trajectoryView.title,
        localizedTitles: trajectoryView.localizedTitles,
        dock: "session",
        component: () =>
          createElement(InstalledComponent, {
            ...trajectorySources,
            scope: "session",
            entry: Trajectory,
          }),
      });
      const diffLifetime = ctx.contributions.child("visual-diff");
      const diffSources = visualComponentProps(runtimeClient(), diffLifetime, "diff");
      const diffView = publishedComponent(diffDeclaration, "diff");
      diffLifetime.add(WORKSPACE_VIEW, {
        id: "package:visual:diff",
        title: diffView.title,
        localizedTitles: diffView.localizedTitles,
        icon: "diff",
        dock: "workspace",
        material: "diff",
        component: () =>
          createElement(InstalledComponent, {
            ...diffSources,
            scope: "workspace",
            entry: WorkspaceDiff,
          }),
      });
      const worktreeSources = visualComponentProps(
        runtimeClient(),
        diffLifetime,
        "worktree-changes",
      );
      contributeLayout(diffLifetime, "chat.header.meta", {
        id: "visual-worktree-changes",
        component: () =>
          createElement(WorkspaceChangesAction, {
            ...worktreeSources,
            targetView: "package:visual:diff",
            entry: WorktreeChanges,
          }),
      });
      const usageLifetime = ctx.contributions.child("visual-usage");
      const usageSources = visualComponentProps(runtimeClient(), usageLifetime, "usage");
      const usageView = publishedComponent(usageDeclaration, "usage");
      usageLifetime.add(WORKSPACE_VIEW, {
        id: "package:visual:usage",
        title: usageView.title,
        localizedTitles: usageView.localizedTitles,
        icon: "chart",
        dock: "workspace",
        component: () =>
          createElement(InstalledComponent, { ...usageSources, scope: "client", entry: Usage }),
      });
      const usageReportsLifetime = ctx.contributions.child("visual-usage-reports");
      const usageReportsSources = visualComponentProps(
        runtimeClient(),
        usageReportsLifetime,
        "usage-reports",
        VISUAL_USAGE_REPORTS_BINDING,
      );
      const usageReportsView = publishedComponent(usageReportsDeclaration, "usage-reports");
      usageReportsLifetime.add(WORKSPACE_VIEW, {
        id: "package:visual:usage-reports",
        title: usageReportsView.title,
        localizedTitles: usageReportsView.localizedTitles,
        icon: "chart",
        dock: "workspace",
        material: "usage",
        component: () =>
          createElement(InstalledComponent, {
            ...usageReportsSources,
            scope: "client",
            entry: UsageReports,
          }),
      });
      const memoryLifetime = ctx.contributions.child("visual-memory");
      const memorySources = visualComponentProps(
        runtimeClient(),
        memoryLifetime,
        "memory",
        VISUAL_MEMORY_BINDING,
      );
      const memoryView = publishedComponent(memoryDeclaration, "memory");
      memoryLifetime.add(WORKSPACE_VIEW, {
        id: "package:visual:memory",
        title: memoryView.title,
        localizedTitles: memoryView.localizedTitles,
        icon: "brain",
        dock: "workspace",
        component: () =>
          createElement(InstalledComponent, {
            ...memorySources,
            scope: "client",
            entry: Memory,
          }),
      });

      const schedulesLifetime = ctx.contributions.child("visual-schedules");
      const schedulesSources = visualComponentProps(
        runtimeClient(),
        schedulesLifetime,
        "schedules",
        VISUAL_SCHEDULES_BINDING,
      );
      const schedulesView = publishedComponent(schedulesDeclaration, "schedules");
      schedulesLifetime.add(WORKSPACE_VIEW, {
        id: "package:visual:schedules",
        title: schedulesView.title,
        localizedTitles: schedulesView.localizedTitles,
        icon: "calendar",
        dock: "workspace",
        component: () =>
          createElement(InstalledComponent, {
            ...schedulesSources,
            scope: "client",
            entry: Schedules,
          }),
      });

      const skillsLifetime = ctx.contributions.child("visual-skills");
      const skillsSources = visualComponentProps(runtimeClient(), skillsLifetime, "skills");
      const skillsView = publishedComponent(skillsDeclaration, "skills");
      skillsLifetime.add(WORKSPACE_VIEW, {
        id: "package:visual:skills",
        title: skillsView.title,
        localizedTitles: skillsView.localizedTitles,
        icon: "book",
        dock: "workspace",
        component: () =>
          createElement(InstalledComponent, {
            ...skillsSources,
            scope: "workspace",
            entry: Skills,
          }),
      });

      ctx.contributions.add(DATA_PROVIDER, {
        key: WORKSPACE_FILES_CHANGED_KEY,
        fetcher: async (_, signal) => {
          const diff = await readDiff(signal);
          if (diff.type !== "visual") throw new Error("Visual fixture requires a visual diff");
          return diff.visual.manifest.map((file): WorkspaceFileChange => ({
            path: file.path,
            change: "mod",
            added: file.added,
            removed: file.removed,
          }));
        },
      });
      let directoryPhase: "initial" | "accepted" | "notified" | "failed" = "initial";
      let directoryQuery: Query | undefined;
      if (directoryState === "refresh-error") {
        const notify = () => {
          if (directoryPhase === "accepted") directoryPhase = "notified";
        };
        // Arm before the entry's bubble listener publishes the fixed Runtime notification.
        window.addEventListener(VISUAL_DIRECTORY_CHANGED_EVENT, notify, true);
        ctx.lifetime.defer(() =>
          window.removeEventListener(VISUAL_DIRECTORY_CHANGED_EVENT, notify, true),
        );
      }
      if (directoryState !== undefined) {
        ctx.lifetime.defer(
          queryClient.getQueryCache().subscribe((event) => {
            if (
              event.type !== "updated" ||
              event.query !== directoryQuery ||
              event.query.state.fetchStatus !== "idle"
            )
              return;
            if (
              event.action.type === "success" &&
              !event.action.manual &&
              directoryPhase !== "notified"
            )
              directoryPhase = "accepted";
            else if (
              event.action.type === "error" &&
              !isCancelledError(event.action.error) &&
              event.query.state.status === "error"
            )
              directoryPhase = "failed";
          }),
        );
      }
      ctx.contributions.add(DATA_PROVIDER, {
        key: WORKSPACE_LIST_FILES_KEY,
        fetcher: async (params, signal) => {
          const request = params as WorkspaceListFilesQuery | undefined;
          if (request?.path === VISUAL_DIRECTORY_PATH) {
            if (
              directoryState !== undefined &&
              request.cwd === VISUAL_WORKSPACE_PATH &&
              request.recursive !== true
            ) {
              directoryQuery = queryClient.getQueryCache().find({
                queryKey: [WORKSPACE_LIST_FILES_KEY, { ...request, cursor: undefined }, "pages"],
                exact: true,
              });
              if (directoryState === "loading") return pending<never>(signal);
              if (directoryState === "empty") return visualFilePage([]);
              if (directoryState === "budget" && !request.cursor)
                return {
                  data: [],
                  nextCursor: "apps-budget",
                  complete: false,
                  coverage: { scannedEntries: 400, excludedEntries: 400, budgetExhausted: true },
                };
              if (directoryState === "paged") {
                const start = request.cursor ? 101 : 1;
                const count = request.cursor ? 20 : 100;
                return {
                  data: Array.from({ length: count }, (_, index): WorkspaceFileEntry => {
                    const name = `page-${String(start + index).padStart(3, "0")}.txt`;
                    return { name, path: `${VISUAL_DIRECTORY_PATH}/${name}`, type: "file" };
                  }),
                  ...(request.cursor ? {} : { nextCursor: "apps-next" }),
                  complete: Boolean(request.cursor),
                  coverage: { scannedEntries: count, excludedEntries: 0, budgetExhausted: false },
                };
              }
              if (
                (directoryState === "error" && directoryPhase === "initial") ||
                (directoryState === "refresh-error" && directoryPhase === "notified")
              )
                throw new Error("Visual fixture could not list apps");
            }
            return visualFilePage([
              {
                path: ACTIVE_DIFF_FILE,
                name: splitFilePath(ACTIVE_DIFF_FILE).name,
                type: "file",
                sizeBytes: 2048,
              },
            ] satisfies WorkspaceFileEntry[]);
          }
          return visualFilePage([
            { path: VISUAL_DIRECTORY_PATH, name: VISUAL_DIRECTORY_PATH, type: "dir" },
            { path: "go.mod", name: "go.mod", type: "file", sizeBytes: 4_096 },
            { path: "README.md", name: "README.md", type: "file", sizeBytes: 2_048 },
            {
              path: "reading-guide.md",
              name: "reading-guide.md",
              type: "file",
              sizeBytes: new TextEncoder().encode(LONG_RESPONSE_MARKDOWN).byteLength,
            },
            {
              path: "reading-blocks.md",
              name: "reading-blocks.md",
              type: "file",
              sizeBytes: new TextEncoder().encode(READING_BLOCKS_MARKDOWN).byteLength,
            },
            {
              path: "reading-callouts.md",
              name: "reading-callouts.md",
              type: "file",
              sizeBytes: new TextEncoder().encode(CALLOUT_MARKDOWN).byteLength,
            },
            ...(state === "dock-file-unsupported" ? [UNSUPPORTED_IMAGE] : []),
          ] satisfies WorkspaceFileEntry[]);
        },
      });

      ctx.contributions.add(DATA_PROVIDER, {
        key: WORKSPACE_READ_FILE_KEY,
        fetcher: async (params) => {
          const path = (params as WorkspaceReadFileQuery).path;
          if (state === "dock-file-error" && path === "README.md") {
            throw new Error("visual fixture could not read README.md");
          }
          if (state === "dock-file-unsupported" && path === UNSUPPORTED_IMAGE.path) {
            throw new RpcError({
              code: -32602,
              message: "visual fixture cannot preview this image",
              data: { type: "unsupported_mime" },
            });
          }
          if (path === "apps/web/package.json") return WEB_PACKAGE_SOURCE;
          if (path === "README.md") return README_SOURCE;
          if (path === "reading-guide.md") return READING_GUIDE_SOURCE;
          if (path === "reading-blocks.md") return READING_BLOCKS_SOURCE;
          if (path === "reading-callouts.md") return READING_CALLOUTS_SOURCE;
          return RESIZER_SOURCE;
        },
      });
      return { mutationLifecycle };
    },
  });
}

function createVisualNotifier(messages: NotificationFeed) {
  return definePlugin({
    name: "flame.visual.notifier",
    setup(ctx) {
      const host = window as unknown as {
        flameVisualNotify?: (message: string, level: NotificationLevel) => void;
      };
      host.flameVisualNotify = (message, level) =>
        messages.notify(level, message, { source: ctx.metadata.pluginName });
      ctx.lifetime.defer(() => delete host.flameVisualNotify);
    },
  });
}

const visualShortcuts = definePlugin({
  name: "flame.visual.shortcuts",
  setup(ctx) {
    ctx.contributions.add(SHORTCUT, {
      key: "Mod+N",
      description: "sidebar.action.newSession",
      handler: () => undefined,
    });
  },
});

async function loadVisualPlugins(plugins: readonly AnyPlugin[]): Promise<void> {
  await loadPluginsForTest(...plugins);
}

const OPENED_BY_ITS_OWN_STATE = new Set([
  "subagents",
  "package:visual:diagnostics",
  "package:visual:memory",
  "package:visual:usage",
  "package:visual:usage-reports",
  "package:visual:schedules",
  "package:visual:skills",
]);

const FULL_VIEW_ID = "file";

export interface VisualWorkspaceConfig {
  sidebarOpen: boolean;
  pane?: VisualSettingsPane;
  fullViewId?: string;
  directoryState?: VisualDirectoryState;
}

export function visualWorkspaceRuntime(
  state: VisualWorkspaceState,
  requestedAgentState?: VisualAgentState,
  reviewFiles?: number,
  reviewPath?: string,
) {
  const agentState =
    requestedAgentState ??
    (state === "dock-light"
      ? "running"
      : state === "dock-runs" || state === "dock-subagents"
        ? "delegated"
        : state === "dock-trajectory"
          ? "tool-shells"
          : "idle");
  const snapshot = structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS[agentState]);
  const capabilities =
    state === "dock-feature-off"
      ? { ...VISUAL_CAPABILITIES, features: { git: VISUAL_CAPABILITIES.features!.git! } }
      : VISUAL_CAPABILITIES;
  let branchEvidenceReads = 0;
  const readDiff = async (signal?: AbortSignal, request?: GetDiffRequest): Promise<Diff> => {
    const input: Pick<GetDiffRequest, "mode" | "path" | "format"> = request ?? {};
    if (state === "dock-loading") return pending<Diff>(signal);
    if (state === "dock-error") throw new Error("Visual fixture could not load the workspace diff");
    if (request && input.format !== "visual")
      throw new Error("Visual fixture only supports the visual diff projection");
    if (state === "dock-diff-evidence") {
      if (input.mode === "base") branchEvidenceReads++;
      return evidenceDiff(input.path, input.mode, branchEvidenceReads > 1);
    }
    if (state === "dock-diff-budget") return request ? budgetDiff() : evidenceDiff();
    if (state === "dock-diff-large") return largeFileDiff(input.path);
    const selection = { firstPath: reviewPath, path: input.path };
    const captured =
      reviewFiles === undefined ? reviewDiff(selection) : scaledReview(reviewFiles, selection);
    const comparison =
      input.mode === "base"
        ? reviewFiles === undefined && !reviewPath
          ? branchReviewDiff(input.path)
          : { ...captured, baseline: BRANCH_REVIEW_DIFF.baseline }
        : captured;
    return state === "dock-empty"
      ? { ...comparison, visual: { ...comparison.visual, manifest: [], patch: "" } }
      : comparison;
  };
  return { agentState, snapshot, capabilities, readDiff };
}

export async function installVisualWorkspaceFixture(
  visual: VisualClient,
  runtimeClient: () => FlameClient,
  state: VisualWorkspaceState,
  theme: VisualWorkspaceTheme,
  { agentState, snapshot, capabilities, readDiff }: ReturnType<typeof visualWorkspaceRuntime>,
  {
    sidebarOpen,
    pane = "appearance",
    fullViewId = FULL_VIEW_ID,
    directoryState,
  }: VisualWorkspaceConfig,
): Promise<void> {
  if (state === "dock-diagnostics-recorded") {
    const { spans, metrics, logs } = structuredClone(VISUAL_DIAGNOSTICS);
    useTelemetryStore.setState({
      spans,
      logs,
      metrics: Object.fromEntries(metrics.map((row) => [row.id, row])),
    });
  }

  await installVisualAgentFixture(
    visual,
    runtimeClient,
    agentState,
    undefined,
    snapshot,
    capabilities,
  );

  const dockViewId =
    state === "dock-catalog"
      ? WORKSPACE_DOCK_CATALOG
      : (DOCK_VIEW_BY_STATE[state] ?? "package:visual:diff");
  const fileViewer =
    state === "dock-files" || state === "dock-catalog"
      ? null
      : state === "dock-file-error"
        ? { path: "README.md", line: 0 }
        : state === "dock-file-unsupported"
          ? { path: UNSUPPORTED_IMAGE.path, line: 0 }
          : { path: ACTIVE_DIFF_FILE, line: 6 };
  visual.workspace.dock.store.setState({
    activeSessionScopeId: VISUAL_SESSION_ID,
    sessionScopes: new Map(),
    dockViewIds:
      state === "dock-catalog"
        ? []
        : [
            ...(OPENED_BY_ITS_OWN_STATE.has(dockViewId) ? [dockViewId] : []),
            "file",
            "package:visual:diff",
            "package:visual:trajectory",
          ],
    lastViewId: state === "dock-catalog" ? null : dockViewId,
    fileFocus: WorkspaceFileFocus.empty().moveTo(ACTIVE_DIFF_FILE),
    fileViewer: null,
    memory: { expandedDirs: [], lastFilePath: null },
    expandedToolIds: new Set(),
  });
  // The viewer owner also records the path and parent folders used when returning to the tree.
  visual.workspace.dock.store.getState().setFileViewer(fileViewer);
  visual.navigation.go({
    session: VISUAL_SESSION_ID,
    dock: dockViewId,
    view: state === "settings" ? "settings" : state === "full-view" ? fullViewId : null,
    settings: state === "settings" ? pane : null,
  });
  visual.theme.store.setState({ theme, visualStyle: "flame" });
  visual.workspace.layout.setState({
    sidebarCollapsed: !sidebarOpen,
    sidebarLayout: null,
    dockLayout:
      state === "dock-catalog"
        ? null
        : {
            dock:
              (state === "dock-review" ? VISUAL_REVIEW_DOCK_WIDTH_RATIO : VISUAL_DOCK_WIDTH_RATIO) *
              100,
            conversation:
              (1 -
                (state === "dock-review"
                  ? VISUAL_REVIEW_DOCK_WIDTH_RATIO
                  : VISUAL_DOCK_WIDTH_RATIO)) *
              100,
          },
  });

  await loadVisualPlugins([
    fileView,
    appearanceSettings,
    createProviderCatalogPlugin(runtimeClient),
    approvalsSettings,
    connectionSettings,
    createInstalledPluginLoader(visual.installedPlugins),
    pluginManagementPane,
    createVisualNotifier(visual.messages),
    visualShortcuts,
    workspaceDataPlugin(readDiff, runtimeClient, state, visual.workspace, directoryState),
    ...(directoryState === "refresh-error"
      ? [createWorkspaceEventsPlugin(visual.agent.access, visual.runtime, runtimeClient)]
      : []),
    visualFileRenderers(runtimeClient),
  ]);

  const root = document.documentElement;
  root.dataset.visualDockWidthCommits = "0";

  visual.workspace.layout.subscribe((next, previous) => {
    if (next.dockLayout === previous.dockLayout) return;
    root.dataset.visualDockWidthCommits = String(
      Number(root.dataset.visualDockWidthCommits ?? "0") + 1,
    );
  });
}

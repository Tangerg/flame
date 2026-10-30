import { IMAGE_FILENAME_STEM } from "@/app/product";
import { createImageExport } from "@/foundation/imageExport";
import { type VisualClient } from "./client";
import { notifyFrom } from "@/features/notifications/public/notifications";
import { InstalledComponent } from "@/features/plugins/ui/InstalledComponent";
import { Schedules } from "../../../plugins/schedules/component/component";
import { visualComponentProps } from "./installedComponentProps";
import { visualFileRenderers } from "./visualFileRenderers";
import { Gallery } from "../../../plugins/icon-gallery/component/component";
import { Diagnostics } from "../../../plugins/diagnostics/component/component";
import { WorkspaceDiff } from "../../../plugins/diff/component/component";
import { type Diff, type GetDiffRequest } from "@flame/runtime-client/wire";
import { Usage } from "../../../plugins/usage/component/component";
import { Memory } from "../../../plugins/memory/component/component";
import { Trajectory } from "../../../plugins/trajectory/component/component";
import { createElement } from "react";
import { createBrowserHost } from "@/platform/browserHost";
import { WORKSPACE_DOCK_CATALOG } from "@/features/workspace/public/navigation";
import { SIDEBAR_DEFAULT_WIDTH_PX } from "@/lib/shellGeometry";
import shortcutsSettings from "@/features/command/shortcuts/index";
import { workbenchSettings } from "@/features/shell/workbench/index";
import appearanceSettings from "@/features/settings/appearance";
import { createBrandPlugin } from "@/features/settings/brand/index";
import { createProvidersPlugin } from "@/features/providers/index";
import approvalsSettings from "@/features/approvals/index";
import connectionSettings from "@/features/connection/settings/index";
import { createHooksPlugin } from "@/features/hooks/index";
import { createMCPServersPlugin } from "@/features/mcp/index";
import personalizationSettings from "@/features/settings/personalization/index";
import { createPluginsPlugin } from "@/features/plugins/index";
import {
  WORKSPACE_FILES_CHANGED_KEY,
  WORKSPACE_LIST_FILES_KEY,
  WORKSPACE_READ_FILE_KEY,
  type WorkspaceFileChange,
  type WorkspaceFileContent,
  type WorkspaceFileEntry,
  type WorkspaceReadFileQuery,
} from "@/features/workspace/application/workspaceQueries";
import { REVIEW_DIFF, BRANCH_REVIEW_DIFF, scaledReview, ACTIVE_DIFF_FILE } from "./diffSnapshot";
import { VISUAL_CAPABILITIES } from "./agentFixtureFacts";
import { VISUAL_MEMORY_BINDING, VISUAL_SCHEDULES_BINDING } from "./componentSnapshots";
import { Skills } from "../../../plugins/skills/component/component";
import { fileView } from "@/features/workspace/views";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { SHORTCUT } from "@/features/command/public/extensionPoints";
import { WORKSPACE_VIEW } from "@/features/workspace/public/views";
import { definePlugin } from "@/extensions/definePlugin";
import { type AnyPlugin } from "dougong";
import { type FlameClient } from "@flame/runtime-client";
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
} from "./workspaceFixtureStates";
const RESIZER_SOURCE: WorkspaceFileContent = {
  startLine: 1,
  endLine: 8,
  totalLines: 8,
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
  endLine: README_CONTENT.split("\n").length,
  totalLines: README_CONTENT.split("\n").length,
};

const READING_GUIDE_SOURCE: WorkspaceFileContent = {
  content: LONG_RESPONSE_MARKDOWN,
  startLine: 1,
  endLine: LONG_RESPONSE_MARKDOWN.split("\n").length,
  totalLines: LONG_RESPONSE_MARKDOWN.split("\n").length,
};

const READING_BLOCKS_SOURCE: WorkspaceFileContent = {
  content: READING_BLOCKS_MARKDOWN,
  startLine: 1,
  endLine: READING_BLOCKS_MARKDOWN.split("\n").length,
  totalLines: READING_BLOCKS_MARKDOWN.split("\n").length,
};

const READING_CALLOUTS_SOURCE: WorkspaceFileContent = {
  content: CALLOUT_MARKDOWN,
  startLine: 1,
  endLine: CALLOUT_MARKDOWN.split("\n").length,
  totalLines: CALLOUT_MARKDOWN.split("\n").length,
};

function pending<T>(signal?: AbortSignal): Promise<T> {
  signal?.throwIfAborted();
  return new Promise<T>((_, reject) => {
    signal?.addEventListener("abort", () => reject(signal.reason), { once: true });
  });
}

function workspaceDataPlugin(
  readDiff: (signal?: AbortSignal) => Promise<Diff>,
  runtimeClient: () => FlameClient,
): AnyPlugin {
  return definePlugin({
    name: "flame.visual.workspace-data",
    setup(ctx) {
      const diagnosticsLifetime = ctx.lifetime("visual-diagnostics");
      const diagnosticsSources = visualComponentProps(
        runtimeClient(),
        diagnosticsLifetime,
        "diagnostics",
      );
      diagnosticsLifetime.contribute(WORKSPACE_VIEW, {
        id: "package:visual:diagnostics",
        title: "Diagnostics",
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
      const galleryLifetime = ctx.lifetime("visual-icon-gallery");
      const gallerySources = visualComponentProps(runtimeClient(), galleryLifetime, "icon-gallery");
      galleryLifetime.contribute(WORKSPACE_VIEW, {
        id: "package:visual:icon-gallery",
        title: "Icon gallery",
        component: () =>
          createElement(InstalledComponent, {
            ...gallerySources,
            scope: "installation",
            entry: Gallery,
          }),
      });
      const lifetime = ctx.lifetime("visual-trajectory");
      const trajectorySources = visualComponentProps(runtimeClient(), lifetime, "trajectory");
      lifetime.contribute(WORKSPACE_VIEW, {
        id: "package:visual:trajectory",
        title: "Session trajectory",
        dock: "session",
        component: () =>
          createElement(InstalledComponent, {
            ...trajectorySources,
            scope: "session",
            entry: Trajectory,
          }),
      });
      const diffLifetime = ctx.lifetime("visual-diff");
      const diffSources = visualComponentProps(runtimeClient(), diffLifetime, "diff");
      diffLifetime.contribute(WORKSPACE_VIEW, {
        id: "package:visual:diff",
        title: "Workspace diff",
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
      const usageLifetime = ctx.lifetime("visual-usage");
      const usageSources = visualComponentProps(runtimeClient(), usageLifetime, "usage");
      usageLifetime.contribute(WORKSPACE_VIEW, {
        id: "package:visual:usage",
        title: "Usage",
        icon: "chart",
        dock: "workspace",
        component: () =>
          createElement(InstalledComponent, { ...usageSources, scope: "client", entry: Usage }),
      });
      const memoryLifetime = ctx.lifetime("visual-memory");
      const memorySources = visualComponentProps(
        runtimeClient(),
        memoryLifetime,
        "memory",
        VISUAL_MEMORY_BINDING,
      );
      memoryLifetime.contribute(WORKSPACE_VIEW, {
        id: "package:visual:memory",
        title: "Agent memory",
        icon: "brain",
        dock: "workspace",
        component: () =>
          createElement(InstalledComponent, {
            ...memorySources,
            scope: "client",
            entry: Memory,
          }),
      });

      const schedulesLifetime = ctx.lifetime("visual-schedules");
      const schedulesSources = visualComponentProps(
        runtimeClient(),
        schedulesLifetime,
        "schedules",
        VISUAL_SCHEDULES_BINDING,
      );
      schedulesLifetime.contribute(WORKSPACE_VIEW, {
        id: "package:visual:schedules",
        title: "Schedules",
        icon: "calendar",
        dock: "workspace",
        component: () =>
          createElement(InstalledComponent, {
            ...schedulesSources,
            scope: "client",
            entry: Schedules,
          }),
      });

      const skillsLifetime = ctx.lifetime("visual-skills");
      const skillsSources = visualComponentProps(runtimeClient(), skillsLifetime, "skills");
      skillsLifetime.contribute(WORKSPACE_VIEW, {
        id: "package:visual:skills",
        title: "Skills",
        icon: "book",
        dock: "workspace",
        component: () =>
          createElement(InstalledComponent, {
            ...skillsSources,
            scope: "workspace",
            entry: Skills,
          }),
      });

      ctx.contribute(DATA_PROVIDER, {
        key: WORKSPACE_FILES_CHANGED_KEY,
        fetcher: async (_, signal) => {
          const diff = await readDiff(signal);
          return (diff.files ?? []).map((file): WorkspaceFileChange => ({
            path: file.path,
            change: "mod",
            added: file.added,
            removed: file.removed,
          }));
        },
      });
      ctx.contribute(DATA_PROVIDER, {
        key: WORKSPACE_LIST_FILES_KEY,
        fetcher: async (params) =>
          (params as { path?: string } | undefined)?.path === "apps"
            ? [{ path: ACTIVE_DIFF_FILE, name: "resizer.ts", type: "file", sizeBytes: 2048 }]
            : ([
                { path: "apps", name: "apps", type: "dir" },
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
              ] satisfies WorkspaceFileEntry[]),
      });

      ctx.contribute(DATA_PROVIDER, {
        key: WORKSPACE_READ_FILE_KEY,
        fetcher: async (params) => {
          const path = (params as WorkspaceReadFileQuery).path;
          if (path === "README.md") return README_SOURCE;
          if (path === "reading-guide.md") return READING_GUIDE_SOURCE;
          if (path === "reading-blocks.md") return READING_BLOCKS_SOURCE;
          if (path === "reading-callouts.md") return READING_CALLOUTS_SOURCE;
          return RESIZER_SOURCE;
        },
      });
    },
  });
}

const visualNotifier = definePlugin({
  name: "flame.visual.notifier",
  setup(ctx) {
    const host = window as unknown as { flameVisualNotify?: (message: string) => void };
    host.flameVisualNotify = (message: string) => notifyFrom(ctx.meta.pluginName, message, "info");
    ctx.cleanup(() => delete host.flameVisualNotify);
  },
});

const visualShortcuts = definePlugin({
  name: "flame.visual.shortcuts",
  setup(ctx) {
    for (const shortcut of [
      {
        key: "Mod+N",
        description: "sidebar.action.newSession",
        handler: () => undefined,
      },
      {
        key: "Escape",
        description: "shortcut.closeWorkspaceView",
        handler: () => undefined,
      },
    ]) {
      ctx.contribute(SHORTCUT, shortcut);
    }
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
  "package:visual:schedules",
  "package:visual:skills",
]);

const FULL_VIEW_ID = "file";

export interface VisualWorkspaceConfig {
  pane?: VisualSettingsPane;
  fullViewId?: string;
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
  const capturedReview = reviewFiles === undefined ? REVIEW_DIFF : scaledReview(reviewFiles);
  const review =
    reviewPath && capturedReview.files
      ? {
          ...capturedReview,
          files: capturedReview.files.map((file, index) =>
            index === 0 ? { ...file, path: reviewPath } : file,
          ),
        }
      : capturedReview;
  const readDiff = async (signal?: AbortSignal, request?: GetDiffRequest): Promise<Diff> => {
    if (state === "dock-loading") return pending<Diff>(signal);
    if (state === "dock-error") throw new Error("Visual fixture could not load the workspace diff");
    const comparison =
      request?.mode === "base"
        ? reviewFiles === undefined && !reviewPath
          ? BRANCH_REVIEW_DIFF
          : { ...review, baseline: BRANCH_REVIEW_DIFF.baseline }
        : review;
    const files =
      state === "dock-empty"
        ? []
        : comparison.files?.filter((file) => !request?.path || file.path === request.path);
    if (request?.format === "raw") {
      return {
        baseline: comparison.baseline,
        patch: files
          ?.map((file) =>
            [
              `diff --git a/${file.previousPath ?? file.path} b/${file.path}`,
              `--- a/${file.previousPath ?? file.path}`,
              `+++ b/${file.path}`,
              ...file.rows.map((row) =>
                row.type === "hunk"
                  ? row.text
                  : `${row.type === "added" ? "+" : row.type === "deleted" ? "-" : " "}${row.code}`,
              ),
              "",
            ].join("\n"),
          )
          .join("\n"),
      };
    }
    return { ...comparison, files };
  };
  return { agentState, snapshot, capabilities, readDiff };
}

export async function installVisualWorkspaceFixture(
  visual: VisualClient,
  runtimeClient: () => FlameClient,
  state: VisualWorkspaceState,
  theme: VisualWorkspaceTheme,
  { agentState, snapshot, capabilities, readDiff }: ReturnType<typeof visualWorkspaceRuntime>,
  { pane = "appearance", fullViewId = FULL_VIEW_ID }: VisualWorkspaceConfig = {},
): Promise<void> {
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
    fileViewer:
      state === "dock-files" || state === "dock-catalog"
        ? null
        : { path: ACTIVE_DIFF_FILE, line: 6 },
    expandedToolIds: new Set(),
  });
  visual.navigation.go({
    session: VISUAL_SESSION_ID,
    dock: dockViewId,
    view: state === "settings" ? "settings" : state === "full-view" ? fullViewId : null,
    settings: state === "settings" ? pane : null,
  });
  visual.theme.store.setState({ theme, visualStyle: "flame" });
  visual.workspace.layout.setState({
    sidebarCollapsed: false,
    sidebarWidth: SIDEBAR_DEFAULT_WIDTH_PX,
    dockWidthRatio:
      state === "dock-catalog"
        ? null
        : state === "dock-review"
          ? VISUAL_REVIEW_DOCK_WIDTH_RATIO
          : VISUAL_DOCK_WIDTH_RATIO,
  });

  await loadVisualPlugins([
    fileView,
    workbenchSettings(visual.workspace.access),
    appearanceSettings,
    createBrandPlugin(createImageExport(IMAGE_FILENAME_STEM, createBrowserHost().saveFile)),
    createProvidersPlugin(runtimeClient, visual.providers.mutations),
    shortcutsSettings(visual.workspace.access),
    approvalsSettings,
    connectionSettings,
    createHooksPlugin(runtimeClient),
    createMCPServersPlugin(runtimeClient),
    personalizationSettings,
    createPluginsPlugin(visual.installedPlugins),
    visualNotifier,
    visualShortcuts,
    workspaceDataPlugin(readDiff, runtimeClient),
    visualFileRenderers(runtimeClient),
  ]);

  const root = document.documentElement;
  root.dataset.visualDockWidthCommits = "0";

  visual.workspace.layout.subscribe((next, previous) => {
    if (next.dockWidthRatio === previous.dockWidthRatio) return;
    root.dataset.visualDockWidthCommits = String(
      Number(root.dataset.visualDockWidthCommits ?? "0") + 1,
    );
  });
}

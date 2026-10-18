import { InstalledComponent } from "@/plugins/builtin/settings/plugins-pane/ui/InstalledComponent";
import { Schedules } from "../../../plugins/schedules/src/component";
import { visualComponentProps } from "./installedComponentProps";
import { visualFileRenderers } from "./visualFileRenderers";
import { Gallery } from "../../../plugins/icon-gallery/src/component";
import { Diagnostics } from "../../../plugins/diagnostics/src/component";
import { WorkspaceDiff } from "../../../plugins/diff/src/component";
import type { Diff } from "@flame/runtime-contract/wire";
import { Usage } from "../../../plugins/usage/src/component";
import { Memory } from "../../../plugins/memory/src/component";
import { Trajectory } from "../../../plugins/trajectory/src/component";
import { createElement } from "react";
import { createBrowserHost } from "@/platform/browserHost";
import { installLocalWorkspaceActions } from "@/plugins/builtin/workspace/adapters/localWorkspaceActions";
import { WORKSPACE_DOCK_CATALOG } from "@/plugins/builtin/workspace/public/navigation";
import { SIDEBAR_DEFAULT_WIDTH_PX } from "@/lib/shellGeometry";
import shortcutsSettings from "@/plugins/builtin/command/shortcuts";
import { workbenchSettings } from "@/plugins/builtin/shell/workbench";
import appearanceSettings from "@/plugins/builtin/settings/appearance";
import { createProvidersPlugin } from "@/plugins/builtin/providers";
import approvalsSettings from "@/plugins/builtin/settings/approvals";
import connectionSettings from "@/plugins/builtin/settings/connection-settings";
import { createHooksPlugin } from "@/plugins/builtin/settings/hooks";
import { createMCPServersPlugin } from "@/plugins/builtin/settings/mcp-servers";
import personalizationSettings from "@/plugins/builtin/settings/personalization";
import { createPluginsPane } from "@/plugins/builtin/settings/plugins-pane";
import { installWorkspaceErrorClassifier } from "@/plugins/builtin/workspace/adapters/runtimeWorkspaceErrorClassifier";
import { installNotificationCentre } from "@/plugins/builtin/shell/status/adapters/systemNotifier";
import {
  WORKSPACE_FILES_CHANGED_KEY,
  WORKSPACE_LIST_FILES_KEY,
  WORKSPACE_READ_FILE_KEY,
  type WorkspaceFileChange,
  type WorkspaceFileContent,
  type WorkspaceFileEntry,
  type WorkspaceReadFileQuery,
} from "@/plugins/builtin/workspace/application/workspaceQueries";
import { REVIEW_DIFF, scaledReview, ACTIVE_DIFF_FILE } from "./diffSnapshot";
import { VISUAL_CAPABILITIES } from "./agentFixtureFacts";
import { VISUAL_MEMORY_BINDING, VISUAL_SCHEDULES_BINDING } from "./componentSnapshots";
import { Skills } from "../../../plugins/skills/src/component";
import { fileView } from "@/plugins/builtin/workspace/views";
import { DATA_PROVIDER, SHORTCUT, WORKSPACE_VIEW, definePlugin } from "@/plugins/sdk";
import type { AnyPlugin } from "dougong";
import type { FlameClient } from "@flame/runtime-contract/client";
import {
  useContextDockStore,
  WorkspaceFileFocus,
} from "@/plugins/builtin/workspace/adapters/contextDockStore";
import { useAppearanceStore } from "@/plugins/builtin/theme/adapters/appearanceStore";
import { useShellLayoutStore } from "@/plugins/builtin/workspace/adapters/shellLayoutStore";
import { navigator } from "@/lib/navigation";
import {
  RUNTIME_AGENT_SESSION_SNAPSHOTS,
  LONG_RESPONSE_MARKDOWN,
  VISUAL_SESSION_ID,
  type VisualAgentState,
} from "./agentSessionSnapshots";
import { installVisualAgentFixture } from "./installVisualAgentFixture";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
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
  totalLines: README_CONTENT.split("\n").length,
};

const READING_GUIDE_SOURCE: WorkspaceFileContent = {
  content: LONG_RESPONSE_MARKDOWN,
  startLine: 1,
  totalLines: LONG_RESPONSE_MARKDOWN.split("\n").length,
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
      ctx.cleanup(installLocalWorkspaceActions(createBrowserHost(), () => false));
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
      galleryLifetime.contribute(WORKSPACE_VIEW, {
        id: "package:visual:icon-gallery",
        title: "Icon gallery",
        component: Gallery,
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
          (params as { path?: string } | undefined)?.path === "app"
            ? [{ path: ACTIVE_DIFF_FILE, name: "resizer.ts", type: "file", sizeBytes: 2048 }]
            : ([
                { path: "app", name: "app", type: "dir" },
                { path: "go.mod", name: "go.mod", type: "file", sizeBytes: 4_096 },
                { path: "README.md", name: "README.md", type: "file", sizeBytes: 2_048 },
                {
                  path: "reading-guide.md",
                  name: "reading-guide.md",
                  type: "file",
                  sizeBytes: new TextEncoder().encode(LONG_RESPONSE_MARKDOWN).byteLength,
                },
              ] satisfies WorkspaceFileEntry[]),
      });

      ctx.contribute(DATA_PROVIDER, {
        key: WORKSPACE_READ_FILE_KEY,
        fetcher: async (params) => {
          const path = (params as WorkspaceReadFileQuery).path;
          if (path === "README.md") return README_SOURCE;
          if (path === "reading-guide.md") return READING_GUIDE_SOURCE;
          return RESIZER_SOURCE;
        },
      });
    },
  });
}

const visualNotifier = definePlugin({
  name: "flame.visual.notifier",
  setup(ctx) {
    const host = window as unknown as { flameVisualNotify?: typeof ctx.notify };
    host.flameVisualNotify = ctx.notify;
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
  const review = reviewFiles === undefined ? REVIEW_DIFF : scaledReview(reviewFiles);
  const readDiff = async (signal?: AbortSignal): Promise<Diff> => {
    if (state === "dock-loading") return pending<Diff>(signal);
    if (state === "dock-error") throw new Error("Visual fixture could not load the workspace diff");
    return state === "dock-empty" ? { baseline: review.baseline } : review;
  };
  return { agentState, snapshot, capabilities, readDiff };
}

export async function installVisualWorkspaceFixture(
  runtimeClient: () => FlameClient,
  state: VisualWorkspaceState,
  theme: VisualWorkspaceTheme,
  { agentState, snapshot, capabilities, readDiff }: ReturnType<typeof visualWorkspaceRuntime>,
  { pane = "appearance", fullViewId = FULL_VIEW_ID }: VisualWorkspaceConfig = {},
): Promise<void> {
  await installVisualAgentFixture(runtimeClient, agentState, undefined, snapshot, capabilities);

  installWorkspaceErrorClassifier();
  installNotificationCentre(createBrowserHost());

  const dockViewId =
    state === "dock-catalog"
      ? WORKSPACE_DOCK_CATALOG
      : (DOCK_VIEW_BY_STATE[state] ?? "package:visual:diff");
  useContextDockStore.setState({
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
  navigator().go({
    session: VISUAL_SESSION_ID,
    dock: dockViewId,
    view: state === "settings" ? "settings" : state === "full-view" ? fullViewId : null,
    settings: state === "settings" ? pane : null,
  });
  useAppearanceStore.setState({ theme, visualStyle: "flame" });
  useShellLayoutStore.setState({
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
    workbenchSettings,
    appearanceSettings,
    createProvidersPlugin(runtimeClient),
    shortcutsSettings,
    approvalsSettings,
    connectionSettings,
    createHooksPlugin(runtimeClient),
    createMCPServersPlugin(runtimeClient),
    personalizationSettings,
    createPluginsPane(runtimeClient, () => "http://visual.test"),
    visualNotifier,
    visualShortcuts,
    workspaceDataPlugin(readDiff, runtimeClient),
    visualFileRenderers(runtimeClient),
  ]);

  const root = document.documentElement;
  root.dataset.visualDockWidthCommits = "0";

  useShellLayoutStore.subscribe((next, previous) => {
    if (next.dockWidthRatio === previous.dockWidthRatio) return;
    root.dataset.visualDockWidthCommits = String(
      Number(root.dataset.visualDockWidthCommits ?? "0") + 1,
    );
  });
}

import { Gallery } from "../../../plugins/icon-gallery/src/component";
import { DiagnosticsReport } from "../../../plugins/diagnostics/src/component";
import { captureClientTelemetry } from "@/lib/observability/clientTelemetry";
import { DiffReport } from "../../../plugins/diff/src/component";
import type { Diff } from "@flame/runtime-contract/wire";
import { UsageReport } from "../../../plugins/usage/src/component";
import { MemoryReport } from "../../../plugins/memory/src/component";
import { TrajectoryReport } from "../../../plugins/trajectory/src/component";
import { createElement } from "react";
import type { TrajectoryEntry, UsageSummary } from "@flame/runtime-contract/wire";
import { createBrowserHost } from "@/platform/browserHost";
import { installLocalWorkspaceActions } from "@/plugins/builtin/workspace/adapters/localWorkspaceActions";
import { WORKSPACE_DOCK_CATALOG } from "@/plugins/builtin/workspace/public/navigation";
import { SIDEBAR_DEFAULT_WIDTH_PX } from "@/lib/shellGeometry";
import shortcutsSettings from "@/plugins/builtin/command/shortcuts";
import { useRuntimeConnectionStore } from "@/plugins/builtin/runtime/adapters/runtimeConnectionProjection";
import { workbenchSettings } from "@/plugins/builtin/shell/workbench";
import appearanceSettings from "@/plugins/builtin/settings/appearance";
import { createProvidersPlugin } from "@/plugins/builtin/providers";
import approvalsSettings from "@/plugins/builtin/settings/approvals";
import connectionSettings from "@/plugins/builtin/settings/connection-settings";
import { createHooksPlugin } from "@/plugins/builtin/settings/hooks";
import { createMCPServersPlugin } from "@/plugins/builtin/settings/mcp-servers";
import personalizationSettings from "@/plugins/builtin/settings/personalization";
import { createPluginsPane } from "@/plugins/builtin/settings/plugins-pane";
import { localePlugins } from "@/plugins/builtin/i18n";
import { installWorkspaceErrorClassifier } from "@/plugins/builtin/workspace/adapters/runtimeWorkspaceErrorClassifier";
import { installNotificationCentre } from "@/plugins/builtin/shell/status/adapters/systemNotifier";
import {
  WORKSPACE_FILES_CHANGED_KEY,
  WORKSPACE_LIST_FILES_KEY,
  WORKSPACE_READ_FILE_KEY,
  WORKSPACE_AGENT_MEMORY_KEY,
  type AgentMemoryEntry,
  type WorkspaceFileChange,
  type WorkspaceFileContent,
  type WorkspaceFileEntry,
} from "@/plugins/builtin/workspace/application/workspaceQueries";
import { visualFeatureCapabilities } from "./agentFixtureFacts";
import { fileView } from "@/plugins/builtin/workspace/views";
import { DATA_PROVIDER, SHORTCUT, WORKSPACE_VIEW, definePlugin } from "@/plugins/sdk";
import type { AnyPlugin } from "dougong";
import type {
  FlameClient,
  FeatureCapability,
  ServerCapabilities,
} from "@flame/runtime-contract/client";
import {
  useContextDockStore,
  WorkspaceFileFocus,
} from "@/plugins/builtin/workspace/adapters/contextDockStore";
import { useAppearanceStore } from "@/plugins/builtin/theme/adapters/appearanceStore";
import { useShellLayoutStore } from "@/plugins/builtin/workspace/adapters/shellLayoutStore";
import { navigator } from "@/lib/navigation";
import {
  RUNTIME_AGENT_SESSION_SNAPSHOTS,
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

const ACTIVE_DIFF_FILE =
  "desktop/frontend/src/plugins/builtin/shell/workbench/panel/DockResizer.tsx";

const REVIEW_DIFF: Diff = {
  baseline: { type: "head", commit: "1234567890abcdef1234567890abcdef12345678" },
  files: [
    {
      path: ACTIVE_DIFF_FILE,
      status: "modified",
      added: 18,
      removed: 6,
      rows: [
        { type: "hunk", text: "@@ -104,8 +104,16 @@ export function DockResizer" },
        {
          type: "context",
          leftLine: 104,
          rightLine: 104,
          code: "const rail = railRef.current;",
        },
        {
          type: "deleted",
          leftLine: 105,
          code: "const width = clampDockWidth(persistedWidth, row.clientWidth);",
        },
        {
          type: "added",
          rightLine: 105,
          code: "const currentWidth = readDockWidth(row);",
        },
        {
          type: "added",
          rightLine: 106,
          code: "const nextWidth = clampDockWidth(currentWidth + delta, row.clientWidth);",
        },
        {
          type: "context",
          leftLine: 106,
          rightLine: 107,
          code: "row.style.setProperty(DOCK_WIDTH_PROPERTY, `${nextWidth}px`);",
        },
        {
          type: "added",
          rightLine: 108,
          code: 'rail.setAttribute("aria-valuenow", String(nextWidth));',
        },
      ],
    },
    {
      path: "runtime/protocol/session.go",
      status: "modified",
      added: 7,
      removed: 3,
      rows: [
        { type: "hunk", text: "@@ -48,5 +48,7 @@ func (session *Session) Commit" },
        {
          type: "context",
          leftLine: 48,
          rightLine: 48,
          code: "func (session *Session) Commit(ctx context.Context) error {",
        },
        {
          type: "deleted",
          leftLine: 49,
          code: "\treturn session.store.Save(ctx, session)",
        },
        {
          type: "added",
          rightLine: 49,
          code: "\tsnapshot := session.snapshot()",
        },
        {
          type: "added",
          rightLine: 50,
          code: "\treturn session.records.Commit(ctx, snapshot)",
        },
        { type: "context", leftLine: 50, rightLine: 51, code: "}" },
      ],
    },
  ],
};

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

function feature(enabled: boolean): FeatureCapability {
  return { enabled, clientOptIn: false, requiredByRunProtocol: false };
}

const VISUAL_CAPABILITIES: ServerCapabilities = {
  runEvents: [],
  runtimeTopics: [],
  features: visualFeatureCapabilities(),
  streamingMethods: [],
  limits: {
    runReplay: { scope: "runtimeInstanceRootSegment", maxEvents: 2_048, maxBytes: 16_777_216 },
    runtimeSubscription: {
      maxTopics: 32,
      maxWatches: 32,
      maxPaths: 256,
      maxDirectoryEntries: 10000,
      maxFileBytes: 1048576,
    },
    idempotency: { namespace: "idp_visual_fixture", retentionSeconds: 86_400 },
    mcpAuthorizationAttempts: { retentionSeconds: 600 },
  },
};

function pending<T>(): Promise<T> {
  return new Promise<T>(() => {});
}

function scaledReview(fileCount: number): Diff {
  const files = Array.from({ length: fileCount }, (_, index) => {
    const long = index % 10 === 0;
    const rows: NonNullable<Diff["files"]>[number]["rows"] = [
      { type: "hunk", text: `@@ -1,24 +1,30 @@ export function module${index}` },
    ];
    for (let line = 1; line <= 24; line++) {
      const code = long
        ? `export const value${line} = ${JSON.stringify("x".repeat(320))};`
        : `export const value${line} = compute(${index}, ${line});`;
      rows.push({ type: "context", leftLine: line, rightLine: line, code });
      if (line % 4 === 0) {
        rows.push({ type: "deleted", leftLine: line, code: `${code} // before` });
        rows.push({ type: "added", rightLine: line, code: `${code} // after` });
      }
    }
    return {
      path: `src/generated/module-${String(index).padStart(3, "0")}.ts`,
      status: "modified" as const,
      added: 6,
      removed: 6,
      rows,
    };
  });
  return { baseline: REVIEW_DIFF.baseline, files };
}

function visualTrajectory(
  snapshot: typeof RUNTIME_AGENT_SESSION_SNAPSHOTS.idle,
): TrajectoryEntry[] {
  const run = snapshot.runs[0]!;
  const entries: TrajectoryEntry[] = [
    ...snapshot.runs.map((value): TrajectoryEntry => ({
      type: "run",
      occurredAt: value.createdAt,
      run: value,
    })),
    ...snapshot.items.map((item): TrajectoryEntry => ({
      type: "item",
      occurredAt: item.type === "toolCall" ? item.startedAt : item.createdAt,
      item,
    })),
    {
      type: "model",
      occurredAt: "2026-07-31T08:00:01.000Z",
      model: {
        callId: "call_visual_inspect",
        runId: run.id,
        segmentId: "segment_visual",
        state: "completed",
        startedAt: "2026-07-31T08:00:01.000Z",
        settledAt: "2026-07-31T08:00:02.420Z",
        firstOutputLatencyMillis: 320,
        usage: { inputTokens: 24_000, outputTokens: 310, cacheReadTokens: 18_000 },
      },
    },
    {
      type: "model",
      occurredAt: "2026-07-31T08:00:06.000Z",
      model: {
        callId: "call_visual_recovered",
        runId: run.id,
        segmentId: "segment_visual",
        state: "unknown",
        startedAt: "2026-07-31T08:00:06.000Z",
        settledAt: "2026-07-31T08:01:00.000Z",
      },
    },
  ];
  return entries.sort((left, right) => Date.parse(right.occurredAt) - Date.parse(left.occurredAt));
}

const visualMemory: AgentMemoryEntry[] = [
  {
    id: "mem_00000000000000000000000000000002",
    scope: "project",
    content: "Prefers the diff read worst-risk-first.",
    origin: "auto",
    status: "pending",
    pinned: false,
    sessionId: VISUAL_SESSION_ID,
    createdAt: "2026-07-30T16:20:00Z",
    updatedAt: "2026-07-30T16:20:00Z",
  },
  {
    id: "mem_00000000000000000000000000000001",
    scope: "project",
    content: "The compaction cutpoint is chosen by the Runtime, never by the client.",
    origin: "auto",
    status: "active",
    pinned: true,
    sessionId: VISUAL_SESSION_ID,
    createdAt: "2026-07-31T10:00:00Z",
    updatedAt: "2026-07-31T10:00:00Z",
  },
];

function workspaceDataPlugin(
  state: VisualWorkspaceState,
  review: Diff,
  snapshot: typeof RUNTIME_AGENT_SESSION_SNAPSHOTS.idle,
): AnyPlugin {
  return definePlugin({
    name: "flame.visual.workspace-data",
    setup(ctx) {
      ctx.cleanup(installLocalWorkspaceActions(createBrowserHost(), () => false));
      const diagnosticsLifetime = ctx.lifetime("visual-diagnostics");
      diagnosticsLifetime.contribute(WORKSPACE_VIEW, {
        id: "package:visual:diagnostics",
        title: "Diagnostics",
        icon: "activity",
        dock: "workspace",
        material: "diagnostics",
        component: () =>
          createElement(DiagnosticsReport, {
            snapshot: captureClientTelemetry(diagnosticsLifetime.signal).read(),
          }),
      });
      const galleryLifetime = ctx.lifetime("visual-icon-gallery");
      galleryLifetime.contribute(WORKSPACE_VIEW, {
        id: "package:visual:icon-gallery",
        title: "Icon gallery",
        dock: "workspace",
        component: Gallery,
      });
      const lifetime = ctx.lifetime("visual-trajectory");
      lifetime.contribute(WORKSPACE_VIEW, {
        id: "package:visual:trajectory",
        title: "Session trajectory",
        dock: "session",
        component: () =>
          createElement(TrajectoryReport, { page: { data: visualTrajectory(snapshot) } }),
      });
      const diffLifetime = ctx.lifetime("visual-diff");
      diffLifetime.contribute(WORKSPACE_VIEW, {
        id: "package:visual:diff",
        title: "Workspace diff",
        icon: "diff",
        dock: "workspace",
        material: "diff",
        component: () =>
          state === "dock-loading"
            ? createElement("p", { role: "status" }, "Reading workspace diff…")
            : state === "dock-error"
              ? createElement(
                  "p",
                  { role: "alert" },
                  "Visual fixture could not load the workspace diff",
                )
              : createElement(DiffReport, {
                  snapshot: {
                    diff: state === "dock-empty" ? { baseline: review.baseline } : review,
                    focusPath: review.files?.[0]?.path,
                  },
                }),
      });
      const usageLifetime = ctx.lifetime("visual-usage");
      usageLifetime.contribute(WORKSPACE_VIEW, {
        id: "package:visual:usage",
        title: "Usage",
        icon: "chart",
        dock: "workspace",
        component: () => createElement(UsageReport, { report: VISUAL_USAGE }),
      });
      const memoryLifetime = ctx.lifetime("visual-memory");
      memoryLifetime.contribute(WORKSPACE_VIEW, {
        id: "package:visual:memory",
        title: "Agent memory",
        icon: "brain",
        dock: "workspace",
        component: () =>
          createElement(MemoryReport, {
            page: {
              Rows: visualMemory.map((item) => ({
                ID: item.id,
                Content: item.content,
                Scope: item.scope,
                Origin: item.origin,
                Status: item.status,
                Pinned: item.pinned,
                SessionID: item.sessionId ?? "",
                Day: "",
                CreatedAt: item.createdAt,
                UpdatedAt: item.updatedAt,
                Project: "",
                Embedding: [],
                EmbeddingSpace: "",
              })),
              NextCursor: "",
            },
            disabled: true,
            edit: () => {},
            command: () => {},
          }),
      });

      ctx.contribute(DATA_PROVIDER, {
        key: WORKSPACE_FILES_CHANGED_KEY,
        fetcher: async () => {
          if (state === "dock-loading") return pending<WorkspaceFileChange[]>();
          if (state === "dock-error") {
            throw new Error("Visual fixture could not load the workspace file changes");
          }
          if (state === "dock-empty") return [];
          return [
            {
              path: "desktop/frontend/src/plugins/DockResizer.tsx",
              change: "mod",
              added: 18,
              removed: 6,
            },
            { path: "runtime/protocol/session.go", change: "mod", added: 7, removed: 3 },
          ] satisfies WorkspaceFileChange[];
        },
      });
      ctx.contribute(DATA_PROVIDER, {
        key: WORKSPACE_AGENT_MEMORY_KEY,
        fetcher: async (): Promise<AgentMemoryEntry[]> => visualMemory,
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
              ] satisfies WorkspaceFileEntry[]),
      });

      ctx.contribute(DATA_PROVIDER, {
        key: WORKSPACE_READ_FILE_KEY,
        fetcher: async () => RESIZER_SOURCE,
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

const VISUAL_USAGE: UsageSummary = {
  total: { inputTokens: 128_400, outputTokens: 41_900, costUsd: 4.12 },
  byProvider: [
    { key: "openai", inputTokens: 96_300, outputTokens: 31_200, costUsd: 3.04, runs: 18 },
    { key: "anthropic", inputTokens: 32_100, outputTokens: 10_700, costUsd: 1.08, runs: 6 },
  ],
  byModel: [
    {
      key: "openai/gpt-5.6-sol",
      inputTokens: 96_300,
      outputTokens: 31_200,
      costUsd: 3.04,
      runs: 18,
    },
    {
      key: "anthropic/claude-opus-5",
      inputTokens: 32_100,
      outputTokens: 10_700,
      costUsd: 1.08,
      runs: 6,
    },
  ],
  sessions: 7,
  runs: 24,
};

const OPENED_BY_ITS_OWN_STATE = new Set([
  "subagents",
  "package:visual:diagnostics",
  "package:visual:memory",
  "package:visual:usage",
]);

const FULL_VIEW_ID = "file";

export interface VisualWorkspaceConfig {
  pane?: VisualSettingsPane;
  fullViewId?: string;
  reviewFiles?: number;
  agentState?: VisualAgentState;
}

export async function installVisualWorkspaceFixture(
  runtimeClient: () => FlameClient,
  state: VisualWorkspaceState,
  theme: VisualWorkspaceTheme,
  {
    pane = "appearance",
    fullViewId = FULL_VIEW_ID,
    reviewFiles,
    agentState: requestedAgentState,
  }: VisualWorkspaceConfig = {},
): Promise<void> {
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
  await installVisualAgentFixture(runtimeClient, agentState, undefined, snapshot);

  installWorkspaceErrorClassifier();
  installNotificationCentre(createBrowserHost());
  useRuntimeConnectionStore.setState({
    capabilities:
      state === "dock-feature-off"
        ? { ...VISUAL_CAPABILITIES, features: { git: feature(true) } }
        : VISUAL_CAPABILITIES,
  });

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
  useAppearanceStore.setState({ theme, visualStyle: "flame", motionScale: 0 });
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
    ...localePlugins,
    appearanceSettings,
    createProvidersPlugin(runtimeClient),
    shortcutsSettings,
    approvalsSettings,
    connectionSettings,
    createHooksPlugin(runtimeClient),
    createMCPServersPlugin(runtimeClient),
    personalizationSettings,
    createPluginsPane(runtimeClient),
    visualNotifier,
    visualShortcuts,
    workspaceDataPlugin(
      state,
      reviewFiles === undefined ? REVIEW_DIFF : scaledReview(reviewFiles),
      snapshot,
    ),
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

import { queryClient } from "@/lib/queryClient";
import { WORKSPACE_DOCK_CATALOG } from "@/plugins/builtin/workspace/public/navigation";
import { SIDEBAR_DEFAULT_WIDTH_PX } from "@/lib/shellGeometry";
import shortcutsSettings from "@/plugins/builtin/command/shortcuts";
import { useRuntimeConnectionStore } from "@/plugins/builtin/runtime/adapters/runtimeConnectionProjection";
import { kernelSettings } from "@/plugins/builtin/shell/kernel";
import appearanceSettings from "@/plugins/builtin/settings/appearance";
import providersSettings from "@/plugins/builtin/settings/providers";
import approvalsSettings from "@/plugins/builtin/settings/approvals";
import brandIconsSettings from "@/plugins/builtin/settings/icon-gallery";
import connectionSettings from "@/plugins/builtin/settings/connection-settings";
import hooksSettings from "@/plugins/builtin/settings/hooks";
import mcpServersSettings from "@/plugins/builtin/settings/mcp-servers";
import personalizationSettings from "@/plugins/builtin/settings/personalization";
import pluginsSettings from "@/plugins/builtin/settings/plugins-pane";
import usageSettings from "@/plugins/builtin/settings/usage";
import { configureUsageGateway } from "@/plugins/builtin/settings/usage/application/ports/usageGateway";
import {
  EMBEDDING_ROLE_KEY,
  PROVIDERS_KEY,
  ProviderConfiguration,
  UTILITY_ROLE_KEY,
} from "@/plugins/builtin/settings/providers/public/queries";
import {
  MCP_SERVERS_KEY,
  type MCPServerSettings,
} from "@/plugins/builtin/settings/mcp-servers/public/serverCatalog";
import { localePlugins } from "@/plugins/builtin/i18n";
import { installWorkspaceErrorClassifier } from "@/plugins/builtin/workspace/adapters/runtimeWorkspaceErrorClassifier";
import {
  WORKSPACE_BUILTIN_TOOLS_KEY,
  WORKSPACE_DIFF_KEY,
  WORKSPACE_FILES_CHANGED_KEY,
  WORKSPACE_LIST_FILES_KEY,
  WORKSPACE_READ_FILE_KEY,
  WORKSPACE_AGENT_DOCS_KEY,
  WORKSPACE_MANAGED_SKILLS_KEY,
  WORKSPACE_RECIPES_KEY,
  WORKSPACE_SKILLS_KEY,
  WORKSPACE_SKILL_PROPOSALS_KEY,
  WORKSPACE_AGENT_MEMORY_KEY,
  WORKSPACE_KNOWLEDGE_KEY,
  type AgentMemoryEntry,
  type BuiltinToolSummary,
  type ManagedSkill,
  type SkillProposal,
  type WorkspaceAgentDoc,
  type WorkspaceRecipe,
  type WorkspaceKnowledgeEntry,
  type WorkspaceSkill,
  type WorkspaceDiff,
  type WorkspaceFileChange,
  type WorkspaceFileContent,
  type WorkspaceFileEntry,
} from "@/plugins/builtin/workspace/application/workspaceQueries";
import { visualFeatureCapabilities } from "./agentFixtureFacts";
import { SCHEDULES_KEY } from "@/plugins/builtin/settings/schedules/application/scheduleQueries";
import type { ScheduleConfig } from "@/plugins/builtin/settings/schedules/application/scheduleConfig";
import {
  diffView,
  fileView,
  inboxView,
  planView,
  timelineView,
  toolsView,
  searchView,
  skillsView,
  recipesView,
  knowledgeView,
  agentMemoryView,
  agentDocsView,
  notificationsView,
} from "@/plugins/builtin/workspace/workspace-views";
import { PENDING_WORK_KEY, type PendingWorkItem } from "@/plugins/builtin/agent/public/hitl";
import { DATA_PROVIDER, SHORTCUT, definePlugin } from "@/plugins/sdk";
import type { AnyPlugin } from "dougong";
import type { FeatureCapability, ServerCapabilities } from "@/rpc";
import {
  useContextDockStore,
  WorkspaceFileFocus,
} from "@/plugins/builtin/workspace/adapters/contextDockStore";
import { useAppearanceStore } from "@/plugins/builtin/theme/adapters/appearanceStore";
import { useShellLayoutStore } from "@/plugins/builtin/workspace/adapters/shellLayoutStore";
import { navigator } from "@/lib/navigation";
import { VISUAL_SESSION_ID } from "./agentSessionSnapshots";
import { installVisualAgentFixture } from "./installVisualAgentFixture";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
import {
  VISUAL_DOCK_WIDTH_RATIO,
  VISUAL_REVIEW_DOCK_WIDTH_RATIO,
  type VisualWorkspaceState,
  type VisualSettingsPane,
  type VisualWorkspaceTheme,
} from "./workspaceFixtureStates";

const ACTIVE_DIFF_FILE = "desktop/frontend/src/plugins/builtin/shell/kernel/panel/DockResizer.tsx";

const REVIEW_DIFF: WorkspaceDiff = {
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

const PROVIDERS: ProviderConfiguration[] = [
  ProviderConfiguration.restore({
    id: "openai",
    baseUrl: "https://api.openai.com/v1",
    credential: { masked: "sk-…7F2A", source: "stored" },
    configured: true,
    credentialRequirement: "apiKeyRequired",
    embeddingCapable: true,
    defaultEmbeddingModel: "text-embedding-3-large",
  }),
  ProviderConfiguration.restore({
    id: "anthropic",
    baseUrl: "https://api.anthropic.com",
    configured: false,
    credentialRequirement: "apiKeyRequired",
    embeddingCapable: false,
  }),
];

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
    runtimeSubscription: { maxTopics: 32, maxWatches: 32 },
    idempotency: { namespace: "idp_visual_fixture", retentionSeconds: 86_400 },
    mcpAuthorizationAttempts: { retentionSeconds: 600 },
  },
};

const VISUAL_SCHEDULES: ScheduleConfig[] = [
  {
    id: "sch_nightly",
    title: "Nightly dependency audit",
    instructions: "Check the lockfile for advisories and open an issue for anything new.",
    cwd: "/Users/visual/scope",
    cron: "0 3 * * *",
    enabled: true,
    revision: 3,
    createdAt: "2026-07-20T09:00:00.000Z",
    nextRunAt: "2026-08-01T03:00:00.000Z",
    lastRunAt: "2026-07-31T03:00:00.000Z",
  },
  {
    id: "sch_weekly",
    title: "Weekly changelog draft",
    instructions: "Summarise the week's merged work into a draft release note.",
    cwd: "/Users/visual/scope",
    cron: "0 9 * * 1",
    enabled: false,
    revision: 1,
    createdAt: "2026-07-06T09:00:00.000Z",
  },
];

function pending<T>(): Promise<T> {
  return new Promise<T>(() => {});
}

function workspaceDataPlugin(state: VisualWorkspaceState): AnyPlugin {
  return definePlugin({
    name: "flame.visual.workspace-data",
    setup(ctx) {
      ctx.contribute(DATA_PROVIDER, {
        key: SCHEDULES_KEY,
        fetcher: async () => VISUAL_SCHEDULES,
      });
      ctx.contribute(DATA_PROVIDER, {
        key: WORKSPACE_DIFF_KEY,
        fetcher: async () => {
          if (state === "dock-loading") return pending<WorkspaceDiff>();
          if (state === "dock-error") {
            throw new Error("Visual fixture could not load the workspace diff");
          }
          if (state === "dock-empty") return { files: [] };
          return REVIEW_DIFF;
        },
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
        key: WORKSPACE_SKILLS_KEY,
        fetcher: async (): Promise<WorkspaceSkill[]> => [
          {
            name: "review-diff",
            description: "Read a change the way a reviewer does, worst risk first.",
            scope: "project",
          },
          {
            name: "write-commit-message",
            description: "State why the change exists, not what the diff already says.",
            scope: "user",
          },
        ],
      });
      ctx.contribute(DATA_PROVIDER, {
        key: WORKSPACE_MANAGED_SKILLS_KEY,
        fetcher: async (): Promise<ManagedSkill[]> => [
          {
            name: "review-diff",
            description: "Read a change the way a reviewer does, worst risk first.",
            lifecycle: "active",
          },
          {
            name: "summarise-standup",
            description: "Superseded by the run digest.",
            lifecycle: "archived",
          },
        ],
      });
      ctx.contribute(DATA_PROVIDER, {
        key: WORKSPACE_SKILL_PROPOSALS_KEY,
        fetcher: async (): Promise<SkillProposal[]> => [
          {
            workspace: "/Users/visual/scope",
            name: "review-diff",
            revision: "rev_02",
            scope: "project",
            description: "Read a change the way a reviewer does, worst risk first.",
            instructions: "Start from the riskiest hunk. Name the invariant it could break.",
            origin: "requested",
            revises: true,
            sourceSession: VISUAL_SESSION_ID,
          },
          {
            workspace: "/Users/visual/scope",
            name: "trace-flaky-test",
            revision: "rev_01",
            scope: "user",
            description: "Find the shared state behind a test that passes alone.",
            instructions: "Run it in isolation, then with its file, then with its package.",
            origin: "mined",
            revises: false,
            sourceSession: VISUAL_SESSION_ID,
          },
        ],
      });
      ctx.contribute(DATA_PROVIDER, {
        key: WORKSPACE_RECIPES_KEY,
        fetcher: async (): Promise<WorkspaceRecipe[]> => [
          {
            name: "review",
            description: "Review the working tree against the base.",
            argumentHint: "[path]",
            body: "Read the diff, then the tests that cover it.",
            scope: "project",
            source: ".flame/recipes/review.md",
          },
          {
            name: "digest",
            body: "Summarise the run for someone who was not watching it.",
            scope: "global",
            source: "~/.flame/recipes/digest.md",
          },
        ],
      });
      ctx.contribute(DATA_PROVIDER, {
        key: WORKSPACE_KNOWLEDGE_KEY,
        fetcher: async (): Promise<WorkspaceKnowledgeEntry[]> => [
          {
            scope: "cwd",
            content: "Run the session suite before touching the store.",
            revision: "rev_07",
            updatedAt: "2026-07-31T10:00:00Z",
          },
          {
            scope: "projectRoot",
            content: "Runtime owns durable semantics; the desktop consumes them.",
            revision: "rev_02",
            updatedAt: "2026-07-24T09:12:00Z",
          },
        ],
      });
      ctx.contribute(DATA_PROVIDER, {
        key: WORKSPACE_AGENT_MEMORY_KEY,
        fetcher: async (): Promise<AgentMemoryEntry[]> => [
          {
            id: "mem_01",
            scope: "project",
            content: "The compaction cutpoint is chosen by the Runtime, never by the client.",
            origin: "auto",
            status: "active",
            pinned: true,
            sessionId: VISUAL_SESSION_ID,
            day: "2026-07-31",
            createdAt: "2026-07-31T10:00:00Z",
            updatedAt: "2026-07-31T10:00:00Z",
          },
          {
            id: "mem_02",
            scope: "user",
            content: "Prefers the diff read worst-risk-first.",
            origin: "user",
            status: "pending",
            pinned: false,
            sessionId: VISUAL_SESSION_ID,
            day: "2026-07-30",
            createdAt: "2026-07-30T16:20:00Z",
            updatedAt: "2026-07-30T16:20:00Z",
          },
        ],
      });
      ctx.contribute(DATA_PROVIDER, {
        key: WORKSPACE_AGENT_DOCS_KEY,
        fetcher: async (): Promise<WorkspaceAgentDoc[]> => [
          { path: "FLAME.md", title: "Workspace instructions", scope: "cwd" },
          { path: "../FLAME.md", title: "Project instructions", scope: "projectRoot" },
          { path: "~/.flame/FLAME.md", title: "Personal instructions", scope: "home" },
        ],
      });
      ctx.contribute(DATA_PROVIDER, {
        key: WORKSPACE_BUILTIN_TOOLS_KEY,
        fetcher: async () =>
          [
            {
              name: "shell",
              description: "Run a shell command",
              parameters: { type: "object", required: ["command"] },
              safetyClass: "exec",
            },
            {
              name: "read_shell_output",
              description: "Read command output",
              parameters: { type: "object" },
              safetyClass: "safe",
            },
            {
              name: "read",
              description: "Read a file",
              parameters: { type: "object", required: ["path"] },
              safetyClass: "safe",
            },
            {
              name: "apply_patch",
              description: "Apply a patch to files",
              parameters: { type: "object" },
              safetyClass: "write",
            },
            {
              name: "grep",
              description: "Search file contents",
              parameters: { type: "object", required: ["query"] },
              safetyClass: "safe",
            },
            {
              name: "glob",
              description: "Find files by name",
              parameters: { type: "object" },
              safetyClass: "safe",
            },
            {
              name: "web_fetch",
              description: "Fetch a page",
              parameters: { type: "object", required: ["url"] },
              safetyClass: "network",
            },
            {
              name: "set_plan",
              description: "Update the Plan",
              parameters: { type: "object" },
              safetyClass: "safe",
            },
            {
              name: "search_memory",
              description: "Search project memory",
              parameters: { type: "object" },
              safetyClass: "safe",
            },
            {
              name: "acme_deploy",
              description: "Ship it (unplaced, from a plugin)",
              parameters: { type: "object" },
            },
          ] satisfies BuiltinToolSummary[],
      });
      ctx.contribute(DATA_PROVIDER, {
        key: MCP_SERVERS_KEY,
        fetcher: async () => [] satisfies MCPServerSettings[],
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
        key: PENDING_WORK_KEY,
        fetcher: async () =>
          state === "dock-inbox"
            ? ([
                {
                  id: "ses_visual:run_root",
                  sessionId: VISUAL_SESSION_ID,
                  rootRunId: "run_root",
                  kind: "approval",
                  subject: "shell",
                  more: 2,
                  waitingSince: "2026-07-31T07:52:00.000Z",
                },
                {
                  id: "ses_other:run_b",
                  sessionId: "ses_other",
                  rootRunId: "run_b",
                  kind: "question",
                  subject: "Which database should the migration target?",
                  more: 0,
                  waitingSince: "2026-07-31T07:58:00.000Z",
                },
              ] satisfies PendingWorkItem[])
            : [],
      });
      ctx.contribute(DATA_PROVIDER, {
        key: PROVIDERS_KEY,
        fetcher: async () => PROVIDERS,
      });
      ctx.contribute(DATA_PROVIDER, {
        key: WORKSPACE_READ_FILE_KEY,
        fetcher: async () => RESIZER_SOURCE,
      });
      ctx.contribute(DATA_PROVIDER, {
        key: UTILITY_ROLE_KEY,
        fetcher: async () => ({ provider: "openai", model: "gpt-5.6" }),
      });
      ctx.contribute(DATA_PROVIDER, {
        key: EMBEDDING_ROLE_KEY,
        fetcher: async () => ({}),
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
  "inbox",
  "tools",
  "recipes",
  "agent-docs",
  "skills",
  "knowledge",
  "agent-memory",
  "notifications",
]);

const FULL_VIEW_ID = "search";

const DOCK_VIEW_BY_STATE: Partial<Record<VisualWorkspaceState, string>> = {
  "dock-light": "plan",
  "dock-inbox": "inbox",
  "dock-timeline": "timeline",
  "dock-runs": "timeline",
  "dock-files": "file",
  "dock-search": "search",
  "dock-recipes": "recipes",
  "dock-agent-docs": "agent-docs",
  "dock-skills": "skills",
  "dock-knowledge": "knowledge",
  "dock-agent-memory": "agent-memory",
  "dock-feature-off": "skills",
  "dock-notifications": "notifications",
  "dock-tools": "tools",
  "dock-file": "file",
  "dock-catalog": WORKSPACE_DOCK_CATALOG,
};

export async function installVisualWorkspaceFixture(
  state: VisualWorkspaceState,
  theme: VisualWorkspaceTheme,
  pane: VisualSettingsPane = "appearance",
  fullViewId: string = FULL_VIEW_ID,
): Promise<void> {
  await installVisualAgentFixture(
    state === "dock-light"
      ? "running"
      : state === "dock-runs"
        ? "delegated"
        : state === "dock-timeline"
          ? "tool-shells"
          : "idle",
  );

  installWorkspaceErrorClassifier();
  useRuntimeConnectionStore.setState({
    capabilities:
      state === "dock-feature-off"
        ? { ...VISUAL_CAPABILITIES, features: { git: feature(true), plan: feature(true) } }
        : VISUAL_CAPABILITIES,
  });
  queryClient.setQueryDefaults([WORKSPACE_DIFF_KEY], {
    retry: false,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  });

  const dockViewId = DOCK_VIEW_BY_STATE[state] ?? "diff";
  useContextDockStore.setState({
    activeSessionScopeId: VISUAL_SESSION_ID,
    sessionScopes: new Map(),
    dockViewIds:
      state === "dock-catalog"
        ? []
        : [
            ...(OPENED_BY_ITS_OWN_STATE.has(dockViewId) ? [dockViewId] : []),
            "file",
            "diff",
            "search",
            "plan",
            "timeline",
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
      state === "dock-review" ? VISUAL_REVIEW_DOCK_WIDTH_RATIO : VISUAL_DOCK_WIDTH_RATIO,
  });

  await loadVisualPlugins([
    workspaceDataPlugin(state),
    diffView,
    fileView,
    inboxView,
    toolsView,
    planView,
    timelineView,
    searchView,
    skillsView,
    recipesView,
    knowledgeView,
    agentMemoryView,
    agentDocsView,
    notificationsView,
    kernelSettings,
    ...localePlugins,
    appearanceSettings,
    providersSettings,
    shortcutsSettings,
    approvalsSettings,
    brandIconsSettings,
    connectionSettings,
    hooksSettings,
    mcpServersSettings,
    personalizationSettings,
    pluginsSettings,
    usageSettings,
    visualNotifier,
    visualShortcuts,
  ]);

  const root = document.documentElement;
  root.dataset.visualDockWidthCommits = "0";
  configureUsageGateway({
    loadSummary: async () => ({
      total: { inputTokens: 128_400, outputTokens: 41_900, costUsd: 4.12 },
      byProvider: [
        { key: "openai", inputTokens: 96_300, outputTokens: 31_200, costUsd: 3.04, runs: 18 },
        { key: "anthropic", inputTokens: 32_100, outputTokens: 10_700, costUsd: 1.08, runs: 6 },
      ],
      byModel: [
        { key: "gpt-5.6-sol", inputTokens: 96_300, outputTokens: 31_200, costUsd: 3.04, runs: 18 },
        { key: "claude-opus-5", inputTokens: 32_100, outputTokens: 10_700, costUsd: 1.08, runs: 6 },
      ],
      sessions: 7,
      runs: 24,
    }),
  });

  useShellLayoutStore.subscribe((next, previous) => {
    if (next.dockWidthRatio === previous.dockWidthRatio) return;
    root.dataset.visualDockWidthCommits = String(
      Number(root.dataset.visualDockWidthCommits ?? "0") + 1,
    );
  });
}

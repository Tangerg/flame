import diagnosticsHTML from "../../../plugins/diagnostics/views/diagnostics.html?raw";
import { Gallery } from "../../../plugins/icon-gallery/src/component";
import { ClientDiagnosticsPackageView } from "@/plugins/builtin/settings/plugins-pane/ui/ClientDiagnosticsPackageView";
import {
  createClientViewReads,
  readClientDiagnostics,
} from "@/plugins/builtin/settings/plugins-pane/adapters/clientViewReads";
import diffHTML from "../../../plugins/diff/views/diff.html?raw";
import { DiffPackageView } from "@/plugins/builtin/settings/plugins-pane/ui/DiffPackageView";
import type { DiffViewTarget } from "@/plugins/builtin/settings/plugins-pane/application/pluginView";
import type { Diff } from "@flame/runtime-contract/wire";
import skillsHTML from "../../../plugins/skills/views/skills.html?raw";
import { SkillsPackageView } from "@/plugins/builtin/settings/plugins-pane/ui/SkillsPackageView";
import type {
  SkillViewTarget,
  SkillViewSnapshot,
} from "@/plugins/builtin/settings/plugins-pane/application/pluginView";
import { UsageReport } from "../../../plugins/usage/src/component";
import scheduleHTML from "../../../plugins/schedules/views/schedules.html?raw";
import scheduleDeclaration from "../../../plugins/schedules/io.github.tangerg.flame/declaration.json";
import { SchedulePackageView } from "@/plugins/builtin/settings/plugins-pane/ui/SchedulePackageView";
import memoryHTML from "../../../plugins/memory/views/memory.html?raw";
import { MemoryPackageView } from "@/plugins/builtin/settings/plugins-pane/ui/MemoryPackageView";
import trajectoryHTML from "../../../plugins/trajectory/views/trajectory.html?raw";
import { TrajectoryPackageView } from "@/plugins/builtin/settings/plugins-pane/ui/TrajectoryPackageView";
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
  WORKSPACE_MANAGED_SKILLS_KEY,
  WORKSPACE_SKILLS_KEY,
  WORKSPACE_SKILL_PROPOSALS_KEY,
  WORKSPACE_AGENT_MEMORY_KEY,
  type AgentMemoryEntry,
  type ManagedSkill,
  type SkillProposal,
  type WorkspaceSkillDiscovery,
  type WorkspaceFileChange,
  type WorkspaceFileContent,
  type WorkspaceFileEntry,
} from "@/plugins/builtin/workspace/application/workspaceQueries";
import { visualFeatureCapabilities } from "./agentFixtureFacts";
import { SCHEDULES_KEY } from "@/plugins/builtin/settings/schedules/application/scheduleQueries";
import type { Schedule } from "@flame/runtime-contract/wire";
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

const VISUAL_SCHEDULES: Schedule[] = [
  {
    id: "sch_nightly",
    title: "Nightly dependency audit",
    instructions: "Check the lockfile for advisories and open an issue for anything new.",
    workspace: { path: "/Users/visual/scope" },
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
    workspace: { path: "/Users/visual/scope" },
    cron: "0 9 * * 1",
    enabled: false,
    revision: 1,
    createdAt: "2026-07-06T09:00:00.000Z",
  },
];

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

const VISUAL_SKILLS: WorkspaceSkillDiscovery = {
  skills: [
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
  diagnostics: [],
};

const VISUAL_MANAGED_SKILLS: ManagedSkill[] = [
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
];

const VISUAL_SKILL_PROPOSALS: SkillProposal[] = [
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
      const diagnosticsReads = () =>
        createClientViewReads(
          { readView: async () => ({ html: diagnosticsHTML }) },
          { installationId: "visual", digest: "visual", viewId: "diagnostics" },
          readClientDiagnostics,
        );
      diagnosticsLifetime.contribute(WORKSPACE_VIEW, {
        id: "package:visual:diagnostics",
        title: "Diagnostics",
        icon: "activity",
        dock: "workspace",
        material: "clientDiagnostics",
        component: () =>
          createElement(ClientDiagnosticsPackageView, {
            reads: diagnosticsReads,
            title: "Diagnostics",
            lifetime: diagnosticsLifetime,
            carrier: createBrowserHost().pluginCarrier,
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
      const read = async () => ({ data: visualTrajectory(snapshot) });
      const reads = () => ({
        binding: { installationId: "fixture", digest: "release", viewId: "view" },
        resource: async () => trajectoryHTML,
        initial: async () => await read(),
        read,
      });
      lifetime.contribute(WORKSPACE_VIEW, {
        id: "package:visual:trajectory",
        title: "Session trajectory",
        dock: "session",
        component: () =>
          createElement(TrajectoryPackageView, {
            reads,
            title: "Session trajectory",
            lifetime,
            carrier: createBrowserHost().pluginCarrier,
          }),
      });
      const diffLifetime = ctx.lifetime("visual-diff");
      const diffReads = (target: DiffViewTarget) => {
        const read = async () => {
          if (state === "dock-loading") return pending<{ diff: Diff; focusPath: string }>();
          if (state === "dock-error")
            throw new Error("Visual fixture could not load the workspace diff");
          return {
            diff: state === "dock-empty" ? { baseline: review.baseline } : review,
            focusPath: target.focusPath,
          };
        };
        return {
          binding: { installationId: "fixture", digest: "release", viewId: "view" },
          resource: async () => diffHTML,
          initial: async () => await read(),
          read,
        };
      };
      diffLifetime.contribute(WORKSPACE_VIEW, {
        id: "package:visual:diff",
        title: "Workspace diff",
        icon: "diff",
        dock: "workspace",
        material: "diff",
        component: () =>
          createElement(DiffPackageView, {
            reads: diffReads,
            title: "Workspace diff",
            lifetime: diffLifetime,
            carrier: createBrowserHost().pluginCarrier,
          }),
      });
      const skillLifetime = ctx.lifetime("visual-skills");
      const skillReads = (target: SkillViewTarget) => {
        const read = async (): Promise<SkillViewSnapshot> => {
          switch (target.type) {
            case "available":
              return { type: target.type, data: VISUAL_SKILLS };
            case "library":
              return { type: target.type, data: { data: VISUAL_MANAGED_SKILLS } };
            case "review":
              return { type: target.type, data: { data: VISUAL_SKILL_PROPOSALS } };
            case "detail": {
              const skill = VISUAL_SKILLS.skills.find((item) => item.name === target.name);
              if (!skill) throw new Error("visual Skill is unavailable");
              return {
                type: target.type,
                data: {
                  ...skill,
                  path: "/runtime/project/skills/" + skill.name + "/SKILL.md",
                  revision: "a".repeat(64),
                  instructions:
                    VISUAL_SKILL_PROPOSALS.find((item) => item.name === skill.name)?.instructions ??
                    "Describe why the change exists.",
                },
              };
            }
          }
        };
        return {
          binding: { installationId: "fixture", digest: "release", viewId: "view" },
          resource: async () => skillsHTML,
          initial: async () => await read(),
          read,
        };
      };
      skillLifetime.contribute(WORKSPACE_VIEW, {
        id: "package:visual:skills",
        title: "Skills",
        icon: "library",
        dock: "workspace",
        component: () =>
          createElement(SkillsPackageView, {
            reads: skillReads,
            title: "Skills",
            lifetime: skillLifetime,
            carrier: createBrowserHost().pluginCarrier,
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
      const scheduleLifetime = ctx.lifetime("visual-schedules");
      const scheduleReads = () => ({
        binding: { installationId: "fixture", digest: "release", viewId: "view" },
        resource: async () => scheduleHTML,
        initial: async () => ({ data: VISUAL_SCHEDULES }),
        read: async () => ({ data: VISUAL_SCHEDULES }),
      });
      scheduleLifetime.contribute(WORKSPACE_VIEW, {
        id: "package:visual:schedules",
        title: "Schedules",
        icon: "calendar-clock",
        dock: "workspace",
        component: () =>
          createElement(SchedulePackageView, {
            reads: scheduleReads,
            templates: scheduleDeclaration.contributes.views[0]!.scheduleTemplates,
            title: "Schedules",
            lifetime: scheduleLifetime,
            carrier: createBrowserHost().pluginCarrier,
          }),
      });
      const memoryLifetime = ctx.lifetime("visual-memory");
      const memoryReads = () => ({
        binding: { installationId: "fixture", digest: "release", viewId: "view" },
        resource: async () => memoryHTML,
        initial: async () => ({ data: visualMemory }),
        read: async () => ({ data: visualMemory }),
      });
      memoryLifetime.contribute(WORKSPACE_VIEW, {
        id: "package:visual:memory",
        title: "Agent memory",
        icon: "brain",
        dock: "workspace",
        component: () =>
          createElement(MemoryPackageView, {
            reads: memoryReads,
            title: "Agent memory",
            lifetime: memoryLifetime,
            carrier: createBrowserHost().pluginCarrier,
          }),
      });

      ctx.contribute(DATA_PROVIDER, {
        key: SCHEDULES_KEY,
        fetcher: async () =>
          VISUAL_SCHEDULES.map(({ workspace, ...schedule }) => ({
            ...schedule,
            cwd: workspace?.path,
          })),
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
        fetcher: async (): Promise<WorkspaceSkillDiscovery> => VISUAL_SKILLS,
      });
      ctx.contribute(DATA_PROVIDER, {
        key: WORKSPACE_MANAGED_SKILLS_KEY,
        fetcher: async (): Promise<ManagedSkill[]> => VISUAL_MANAGED_SKILLS,
      });
      ctx.contribute(DATA_PROVIDER, {
        key: WORKSPACE_SKILL_PROPOSALS_KEY,
        fetcher: async (): Promise<SkillProposal[]> => VISUAL_SKILL_PROPOSALS,
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
  "package:visual:skills",
  "package:visual:memory",
  "package:visual:schedules",
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
    createPluginsPane(runtimeClient, createBrowserHost().pluginCarrier),
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

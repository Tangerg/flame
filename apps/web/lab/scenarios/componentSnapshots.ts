import { type TelemetrySnapshot } from "@flame/component-sdk";
import { pluginContract } from "@flame/runtime-client";
import {
  type UsageSummary,
  type PluginOperationBinding,
  type PluginInstallation,
  type PluginProviderSelection,
} from "@flame/runtime-client/wire";
import { CAPABILITY_JSON as MEMORY_CAPABILITY } from "../../../../plugins/memory/package/io.github.tangerg.flame/contracts/methods.generated";
import { CAPABILITY_JSON as SCHEDULES_CAPABILITY } from "../../../../plugins/schedules/package/io.github.tangerg.flame/contracts/methods.generated";
import { CAPABILITY_JSON as USAGE_REPORTS_CAPABILITY } from "../../../../plugins/usagereports/component/methods.generated";
import { VISUAL_PLUGIN_DIGEST, VISUAL_WORKSPACE_PATH } from "./agentFixtureFacts";
import { type MethodOutputs as MemoryOutputs } from "../../../../plugins/memory/package/io.github.tangerg.flame/contracts/methods.generated";
import { type MethodOutputs as ScheduleOutputs } from "../../../../plugins/schedules/package/io.github.tangerg.flame/contracts/methods.generated";
import { type MethodOutputs as UsageReportOutputs } from "../../../../plugins/usagereports/component/methods.generated";
import { VISUAL_SESSION_ID } from "./agentSessionSnapshots";
const visualMemory: MemoryOutputs["flame.memory/list.v2"]["Rows"] = [
  {
    Revision: 3,
    ID: "mem_00000000000000000000000000000002",
    Scope: "project",
    Day: "",
    Project: "",
    Embedding: [],
    EmbeddingSpace: "",
    Content: "Prefers the diff read worst-risk-first.",
    Origin: "auto",
    Status: "pending",
    Pinned: false,
    SessionID: VISUAL_SESSION_ID,
    CreatedAt: "2026-07-30T16:20:00Z",
    UpdatedAt: "2026-07-30T16:20:00Z",
  },
  {
    Revision: 7,
    ID: "mem_00000000000000000000000000000001",
    Scope: "project",
    Day: "",
    Project: "",
    Embedding: [],
    EmbeddingSpace: "",
    Content: "The compaction cutpoint is chosen by the Runtime, never by the client.",
    Origin: "auto",
    Status: "active",
    Pinned: true,
    SessionID: VISUAL_SESSION_ID,
    CreatedAt: "2026-07-31T10:00:00Z",
    UpdatedAt: "2026-07-31T10:00:00Z",
  },
];

export const VISUAL_MEMORY_PAGE: MemoryOutputs["flame.memory/list.v2"] = {
  Rows: visualMemory,
  NextCursor: "",
  Capacity: 512,
};

export const VISUAL_USER_MEMORY_PAGE: MemoryOutputs["flame.memory/list.v2"] = {
  ...VISUAL_MEMORY_PAGE,
  Rows: [
    {
      Revision: 2,
      ID: "mem_00000000000000000000000000000003",
      Scope: "user",
      Day: "",
      Project: "",
      Embedding: [],
      EmbeddingSpace: "",
      Content: "Prefers concise summaries with source paths and concrete next steps.",
      Origin: "user",
      Status: "active",
      Pinned: false,
      SessionID: "",
      CreatedAt: "2026-07-31T10:00:00Z",
      UpdatedAt: "2026-07-31T10:00:00Z",
    },
  ],
};

type VisualMemoryRow = MemoryOutputs["flame.memory/list.v2"]["Rows"][number];

const projectMemoryRecord: Pick<
  VisualMemoryRow,
  "Scope" | "Day" | "Project" | "Embedding" | "EmbeddingSpace"
> = {
  Scope: "project",
  Day: "",
  Project: "",
  Embedding: [],
  EmbeddingSpace: "",
};

export const VISUAL_LONG_MEMORY_PAGE: MemoryOutputs["flame.memory/list.v2"] = {
  ...VISUAL_MEMORY_PAGE,
  Rows: [
    {
      ...projectMemoryRecord,
      Revision: 1,
      ID: "mem_00000000000000000000000000000010",
      Content: [
        "Review findings are reported by risk, then grouped by file.",
        "",
        "Each finding cites the changed line range and the exact command output that supports it; a summary never replaces the recorded evidence.",
      ].join("\n"),
      Origin: "auto",
      Status: "pending",
      Pinned: false,
      SessionID: VISUAL_SESSION_ID,
      CreatedAt: "2026-07-31T09:12:00Z",
      UpdatedAt: "2026-07-31T09:12:00Z",
    },
    {
      ...projectMemoryRecord,
      Revision: 4,
      ID: "mem_00000000000000000000000000000011",
      Content: [
        "工作区的格式检查统一使用仓库根目录的配置，不要在子包内另行添加格式化规则。",
        "",
        "",
        "检查失败时先保留原始输出，再说明受影响的文件和行号；不要只给出“已修复”之类的结论。",
      ].join("\n"),
      Origin: "auto",
      Status: "active",
      Pinned: true,
      SessionID: VISUAL_SESSION_ID,
      CreatedAt: "2026-07-29T15:40:00Z",
      UpdatedAt: "2026-07-30T08:05:00Z",
    },
    {
      ...projectMemoryRecord,
      Revision: 1,
      ID: "mem_00000000000000000000000000000012",
      Content: [
        "The failing fixture output from the last compaction review is retained at:",
        "",
        `${VISUAL_WORKSPACE_PATH}/.flame/evidence/${"recorded-compaction-boundary-".repeat(6)}trajectory.jsonl`,
        "",
        "Re-read that file before proposing a fix.",
      ].join("\n"),
      Origin: "auto",
      Status: "pending",
      Pinned: false,
      SessionID: VISUAL_SESSION_ID,
      CreatedAt: "2026-07-31T11:48:00Z",
      UpdatedAt: "2026-07-31T11:48:00Z",
    },
    {
      ...projectMemoryRecord,
      Revision: 3,
      ID: "mem_00000000000000000000000000000013",
      Content: [
        "Release notes for this workspace are written in English and Chinese.",
        "",
        "Keep both versions in the same entry so reviewers can compare the wording line by line. 中英文版本放在同一条目中，便于逐行对照。",
      ].join("\n"),
      Origin: "user",
      Status: "active",
      Pinned: true,
      SessionID: "",
      CreatedAt: "2026-07-29T10:00:00Z",
      UpdatedAt: "2026-07-30T17:25:00Z",
    },
    {
      ...projectMemoryRecord,
      Revision: 5,
      ID: "mem_00000000000000000000000000000016",
      Content:
        "Diff review reads the worktree comparison first and the branch comparison only when the user asks for it. Renamed files are reviewed under their new path, with the previous path kept beside the heading so earlier comments still resolve. Generated resources, lockfiles and snapshots are listed but not reviewed line by line unless their change is unexpected for the task; when that happens the review states which source change should have produced them and whether it is present in the same diff.",
      Origin: "auto",
      Status: "active",
      Pinned: false,
      SessionID: VISUAL_SESSION_ID,
      CreatedAt: "2026-07-29T16:20:00Z",
      UpdatedAt: "2026-07-31T09:40:00Z",
    },
    ...VISUAL_MEMORY_PAGE.Rows,
  ],
};

const userMemoryRecord: Pick<
  VisualMemoryRow,
  "Scope" | "Day" | "Project" | "Embedding" | "EmbeddingSpace" | "Origin" | "Status" | "SessionID"
> = {
  Scope: "user",
  Day: "",
  Project: "",
  Embedding: [],
  EmbeddingSpace: "",
  Origin: "user",
  Status: "active",
  SessionID: "",
};

export const VISUAL_LONG_USER_MEMORY_PAGE: MemoryOutputs["flame.memory/list.v2"] = {
  ...VISUAL_USER_MEMORY_PAGE,
  Rows: [
    {
      ...userMemoryRecord,
      Revision: 3,
      ID: "mem_00000000000000000000000000000020",
      Content: [
        "Answer in the language of the question.",
        "",
        "When a question mixes English and Chinese, reply in Chinese and keep code identifiers, paths and commands in their original form.",
      ].join("\n"),
      Pinned: true,
      CreatedAt: "2026-07-28T09:00:00Z",
      UpdatedAt: "2026-07-30T11:15:00Z",
    },
    {
      ...userMemoryRecord,
      Revision: 1,
      ID: "mem_00000000000000000000000000000021",
      Content: [
        "回答时先给结论，再列出依据。",
        "",
        "涉及文件时写出完整路径；涉及命令时给出可以直接复制的形式，并说明预期输出。",
      ].join("\n"),
      Pinned: false,
      CreatedAt: "2026-07-29T08:30:00Z",
      UpdatedAt: "2026-07-29T08:30:00Z",
    },
    {
      ...userMemoryRecord,
      Revision: 4,
      ID: "mem_00000000000000000000000000000023",
      Content:
        "Never report a task as finished while a check is failing or was skipped. State which checks ran, which were not run and why, and paste the relevant failing output verbatim instead of paraphrasing it. If a failure predates the change, say so and show how that was established, for example by running the same check on the unchanged revision.",
      Pinned: true,
      CreatedAt: "2026-07-28T17:45:00Z",
      UpdatedAt: "2026-07-31T08:20:00Z",
    },
    {
      ...userMemoryRecord,
      Revision: 1,
      ID: "mem_00000000000000000000000000000024",
      Content: [
        "提交说明使用英文，第一行不超过七十二个字符。",
        "",
        "",
        "正文说明改动原因而不是复述差异；如果改动涉及界面，附上前后对比的截图路径。",
      ].join("\n"),
      Pinned: false,
      CreatedAt: "2026-07-30T15:00:00Z",
      UpdatedAt: "2026-07-30T15:00:00Z",
    },
    ...VISUAL_USER_MEMORY_PAGE.Rows,
  ],
};

export const VISUAL_SCHEDULE_INSTRUCTION_LINES = [
  "Inspect workspace dependencies.",
  "Run accepted checks.",
  "",
  "Report:",
  "- State concrete risks.",
  "- Preserve exact output.",
  "",
  `/Users/visual/${"retained-evidence-".repeat(8)}request.json`,
] as const;

export const VISUAL_SCHEDULES_PAGE: ScheduleOutputs["flame.schedules/list.v1"] = {
  Rows: [
    {
      ID: "sch_nightly",
      Revision: 1,
      Title: "Nightly dependency audit",
      Instructions: VISUAL_SCHEDULE_INSTRUCTION_LINES.join("\n"),
      Cron: "0 2 * * *",
      TimeZone: "UTC",
      CWD: "/Users/visual/scope",
      ModelSelection: { provider: "openai", model: "gpt-5.6-sol", reasoningEffort: "medium" },
      CreatedAt: "2026-07-29T08:00:00Z",
      LastRunAt: "2026-07-31T02:00:00Z",
      NextRunAt: "2026-08-01T02:00:00Z",
    },
    {
      ID: "sch_weekly",
      Revision: 3,
      Title: "Weekly maintenance notes",
      Instructions: "Summarize maintenance work and open questions.",
      Cron: "0 9 * * 1",
      TimeZone: "UTC",
      CWD: "/Users/visual/scope",
      ModelSelection: { provider: "", model: "", reasoningEffort: "" },
      CreatedAt: "2026-07-28T08:00:00Z",
    },
  ],
  NextCursor: "",
};

export const VISUAL_MEMORY_BINDING: PluginOperationBinding = {
  installationId: "11111111-1111-4111-8111-111111111111",
  digest: VISUAL_PLUGIN_DIGEST,
  contribution: "memory-provider",
  contract: MEMORY_CAPABILITY,
};

export const VISUAL_SCHEDULES_BINDING: PluginOperationBinding = {
  installationId: "22222222-2222-4222-8222-222222222222",
  digest: VISUAL_PLUGIN_DIGEST,
  contribution: "schedules-provider",
  contract: SCHEDULES_CAPABILITY,
};

export const VISUAL_USAGE_REPORTS_BINDING: PluginOperationBinding = {
  installationId: "44444444-4444-4444-8444-444444444444",
  digest: VISUAL_PLUGIN_DIGEST,
  contribution: "usage-reports",
  contract: USAGE_REPORTS_CAPABILITY,
};

const visualAllUsage = {
  total: { inputTokens: 128_400, outputTokens: 41_900, costUsd: 4.12 },
  recordedCostUsd: 4.12,
  coverage: {
    calls: 24,
    reportedCalls: 24,
    pricedCalls: 24,
    startedCalls: 0,
    completedCalls: 24,
    failedCalls: 0,
    unknownCalls: 0,
  },
};

const visualOpenAIUsage = {
  total: { inputTokens: 96_300, outputTokens: 31_200, costUsd: 3.04 },
  recordedCostUsd: 3.04,
  coverage: {
    calls: 18,
    reportedCalls: 18,
    pricedCalls: 18,
    startedCalls: 0,
    completedCalls: 18,
    failedCalls: 0,
    unknownCalls: 0,
  },
};

const visualAnthropicUsage = {
  total: { inputTokens: 32_100, outputTokens: 10_700, costUsd: 1.08 },
  recordedCostUsd: 1.08,
  coverage: {
    calls: 6,
    reportedCalls: 6,
    pricedCalls: 6,
    startedCalls: 0,
    completedCalls: 6,
    failedCalls: 0,
    unknownCalls: 0,
  },
};

export const VISUAL_USAGE: UsageSummary = {
  observedAt: "2026-10-20T00:00:00Z",
  ...visualAllUsage,
  byProvider: [
    { key: "openai", ...visualOpenAIUsage },
    { key: "anthropic", ...visualAnthropicUsage },
  ],
  byModel: [
    { key: "openai/gpt-5.6-sol", ...visualOpenAIUsage },
    { key: "anthropic/claude-opus-5", ...visualAnthropicUsage },
  ],
  byDay: [
    { key: "2026-10-18", ...visualOpenAIUsage },
    { key: "2026-10-19", ...visualAnthropicUsage },
  ],
  byOrigin: [
    { key: "automatic", ...visualAnthropicUsage },
    { key: "human", ...visualOpenAIUsage },
  ],
  byScope: [{ key: "run", ...visualAllUsage }],
  sessions: 7,
  runs: 24,
};

const visualSavedDailyUsage = {
  total: { inputTokens: 9_007_199_254_740_993n, outputTokens: 0 },
  recordedCostUsd: 0,
  coverage: {
    calls: 6,
    reportedCalls: 6,
    pricedCalls: 5,
    startedCalls: 0,
    completedCalls: 6,
    failedCalls: 0,
    unknownCalls: 0,
  },
};

export const VISUAL_USAGE_REPORTS: readonly UsageReportOutputs["flame.usage-reports/get.v1"][] = [
  {
    id: "b".repeat(64),
    period: "weekly",
    observedAt: "2026-10-20T09:00:00Z",
    since: "2026-10-13T09:00:00Z",
    summary: {
      ...VISUAL_USAGE,
      observedAt: "2026-10-20T09:00:00Z",
      since: "2026-10-13T09:00:00Z",
    },
  },
  {
    id: "a".repeat(64),
    period: "daily",
    observedAt: "2026-10-20T08:00:00Z",
    since: "2026-10-19T08:00:00Z",
    summary: {
      observedAt: "2026-10-20T08:00:00Z",
      since: "2026-10-19T08:00:00Z",
      ...visualSavedDailyUsage,
      byProvider: [{ key: "anthropic", ...visualSavedDailyUsage }],
      byModel: [{ key: "anthropic/claude-opus-5", ...visualSavedDailyUsage }],
      byDay: [{ key: "2026-10-19", ...visualSavedDailyUsage }],
      byOrigin: [{ key: "automatic", ...visualSavedDailyUsage }],
      byScope: [{ key: "run", ...visualSavedDailyUsage }],
      sessions: 2,
      runs: 6,
    },
  },
];

const VISUAL_TRACE_ID = "0123456789abcdef0123456789abcdef";

const VISUAL_SPAN_ATTRS = {
  source: "workspace.preview",
  path: "/Users/visual/scope/apps/web/src/features/workspace/ui/files/FileWorkspace.tsx",
};

export const VISUAL_DIAGNOSTICS: TelemetrySnapshot = {
  spans: [
    {
      id: "0000000000000001",
      traceId: VISUAL_TRACE_ID,
      name: "workspace.preview.read",
      kind: "internal",
      startMs: 1_785_484_800_000,
      durationMillis: 3,
      status: "unset",
      attrs: VISUAL_SPAN_ATTRS,
    },
    {
      id: "0000000000000002",
      traceId: VISUAL_TRACE_ID,
      parentSpanId: "0000000000000001",
      name: "workspace.preview.decode",
      kind: "internal",
      startMs: 1_785_484_801_000,
      durationMillis: 21.5,
      status: "ok",
      attrs: VISUAL_SPAN_ATTRS,
    },
    {
      id: "0000000000000003",
      traceId: VISUAL_TRACE_ID,
      parentSpanId: "0000000000000001",
      name: "workspace.preview.refresh",
      kind: "internal",
      startMs: 1_785_484_802_000,
      durationMillis: 42,
      status: "error",
      statusMessage: "The source changed after the accepted preview request.",
      attrs: VISUAL_SPAN_ATTRS,
    },
  ],
  metrics: [
    {
      id: "workspace.preview.latency",
      name: "workspace.preview.latency",
      description: "Preview decoding duration",
      kind: "histogram",
      unit: "ms",
      attrs: { source: "workspace.preview" },
      count: 12,
      sum: 840,
      p50: 50,
      p95: 100,
      avg: 70,
    },
    {
      id: "workspace.preview.opens",
      name: "workspace.preview.opens",
      description: "Accepted preview opens",
      kind: "counter",
      unit: "{preview}",
      attrs: { source: "workspace.preview" },
      count: 24,
      sum: 24,
    },
  ],
  logs: [
    {
      id: "provider-notice",
      timeMs: 1_785_484_802_000,
      severity: "PROVIDER_NOTICE",
      body: "\n  Provider notice: cache warmed.\n\n    detail: 3 entries\n",
      attrs: { source: "workspace.preview" },
    },
    {
      id: "reading-log",
      timeMs: 1_785_484_803_000,
      severity: "ERROR",
      body: [
        "",
        "Workspace preview could not be refreshed.",
        "",
        "Error: the file changed after the accepted request.",
        "    at refresh (/Users/visual/scope/apps/web/src/features/workspace/ui/files/FileWorkspace.tsx:89:7)",
        "The last accepted preview remains available.",
      ].join("\n"),
      traceId: VISUAL_TRACE_ID,
      spanId: "0000000000000003",
      attrs: { source: "workspace.preview" },
    },
  ],
};

export const VISUAL_PACKAGES: PluginInstallation[] = (
  [
    [VISUAL_MEMORY_BINDING, "flame.memory", "memory", "memory"],
    [VISUAL_SCHEDULES_BINDING, "flame.schedules", "schedules", "schedules"],
    [VISUAL_USAGE_REPORTS_BINDING, "flame.usagereports", "usagereports", "reports"],
  ] satisfies readonly (readonly [PluginOperationBinding, string, string, string])[]
).map(([operation, name, server, exportName]) => {
  return {
    id: operation.installationId,
    source: `/visual/packages/${name}`,
    enabled: true,
    inputStates: {},
    disabledServers: [],
    disabledSkills: [],
    realization: { type: "available" },
    presentation: "withheld",
    selected: {
      digest: operation.digest,
      name,
      version: "2.0.0",
      contributions: [
        {
          id: operation.contribution,
          server,
          export: exportName,
          contract: operation.contract,
          selectionRoles: [pluginContract(operation.contract).reference],
          requires: [],
        },
      ],
      points: [],
      pointValues: [],
      terminalViews: [],
      components: [],
      servers: [{ type: "stdio", name: server, command: `/visual/bin/${server}` }],
      inputs: [],
      skills: [],
      locales: [],
      themes: [],
      diagnostics: [],
    },
  };
});

export const VISUAL_PLUGIN_PROVIDERS: PluginProviderSelection[] = VISUAL_PACKAGES.map(
  (installation) => ({
    role: installation.selected.contributions[0]!.selectionRoles![0]!,
    installationId: installation.id,
    contribution: installation.selected.contributions[0]!.id,
  }),
);

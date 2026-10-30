import { pluginContract } from "@flame/runtime-client";
import {
  type UsageSummary,
  type PluginOperationBinding,
  type PluginInstallation,
  type PluginProviderSelection,
} from "@flame/runtime-client/wire";
import { CAPABILITY_JSON as MEMORY_CAPABILITY } from "../../../plugins/memory/package/io.github.tangerg.flame/contracts/methods.generated";
import { CAPABILITY_JSON as SCHEDULES_CAPABILITY } from "../../../plugins/schedules/package/io.github.tangerg.flame/contracts/methods.generated";
import { VISUAL_PLUGIN_DIGEST } from "./agentFixtureFacts";
import { type MethodOutputs as MemoryOutputs } from "../../../plugins/memory/package/io.github.tangerg.flame/contracts/methods.generated";
import { type MethodOutputs as ScheduleOutputs } from "../../../plugins/schedules/package/io.github.tangerg.flame/contracts/methods.generated";
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

export const VISUAL_SCHEDULES_PAGE: ScheduleOutputs["flame.schedules/list.v1"] = {
  Rows: [
    {
      ID: "sch_nightly",
      Revision: 1,
      Title: "Nightly dependency audit",
      Instructions: "Review workspace dependencies and summarize risks.",
      Cron: "0 2 * * *",
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
      CWD: "/Users/visual/scope",
      ModelSelection: { provider: "", model: "", reasoningEffort: "" },
      CreatedAt: "2026-07-28T08:00:00Z",
      LastRunAt: "0001-01-01T00:00:00Z",
      NextRunAt: "0001-01-01T00:00:00Z",
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

export const VISUAL_USAGE: UsageSummary = {
  observedAt: "2026-10-20T00:00:00Z",
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
  byProvider: [
    {
      key: "openai",
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
    },
    {
      key: "anthropic",
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
    },
  ],
  byModel: [
    {
      key: "openai/gpt-5.6-sol",
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
    },
    {
      key: "anthropic/claude-opus-5",
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
    },
  ],
  byDay: [],
  byOrigin: [],
  byScope: [],
  sessions: 7,
  runs: 24,
};

export const VISUAL_PACKAGES: PluginInstallation[] = (
  [
    [VISUAL_MEMORY_BINDING, "flame.memory", "memory"],
    [VISUAL_SCHEDULES_BINDING, "flame.schedules", "schedules"],
  ] satisfies readonly (readonly [PluginOperationBinding, string, string])[]
).map(([operation, name, server]) => {
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
          export: server,
          contract: operation.contract,
          requires: [],
        },
      ],
      points: [],
      pointValues: [],
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
    role: pluginContract(installation.selected.contributions[0]!.contract).reference,
    installationId: installation.id,
    contribution: installation.selected.contributions[0]!.id,
  }),
);

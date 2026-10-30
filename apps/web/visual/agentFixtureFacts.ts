import { RuntimeConnectionGeneration } from "@/features/connection/public/services";
import { type ServerCapabilities } from "@flame/runtime-client/wire";
export const VISUAL_PRIMARY_MODEL_CONTEXT_WINDOW = 1_050_000;

export const VISUAL_CONTEXT_TOKENS = 96_000;

export const VISUAL_WORKSPACE_PATH = "/Users/visual/scope";

export const VISUAL_NOW = Date.parse("2026-07-31T14:30:00Z");

export const VISUAL_RUNTIME_FEATURES = [
  "git",
  "mcp",
  "fileWatch",
  "plan",
  "skills",
  "plugins",
  "relocate",
  "subagents",
  "sessionExport",
] as const;

function visualFeatureCapabilities(): Record<
  string,
  { enabled: boolean; clientOptIn: boolean; requiredByRunProtocol: boolean }
> {
  return Object.fromEntries(
    VISUAL_RUNTIME_FEATURES.map((name) => [
      name,
      { enabled: true, clientOptIn: false, requiredByRunProtocol: false },
    ]),
  );
}

export const VISUAL_RUNTIME_GENERATION = RuntimeConnectionGeneration.forProcess(
  "visual-runtime-connection",
);

export const VISUAL_CAPABILITIES: ServerCapabilities = {
  runEvents: [],
  runtimeTopics: [
    "files.changed",
    "skills.changed",
    "plugins.changed",
    "plugins.dataChanged",
    "mcp.changed",
    "approvals.changed",
  ],
  features: visualFeatureCapabilities(),
  streamingMethods: ["runtime.subscribe"],
  limits: {
    runReplay: { scope: "runtimeInstanceRootSegment", maxEvents: 2_048, maxBytes: 16_777_216 },
    runtimeSubscription: {
      maxTopics: 32,
      maxWatches: 32,
      maxPaths: 256,
      maxDirectoryEntries: 10000,
      maxFileBytes: 1048576,
    },
    idempotency: { namespace: "idp_11111111111111111111111111111111", retentionSeconds: 86_400 },
    mcpAuthorizationAttempts: { retentionSeconds: 600 },
  },
};

export const VISUAL_PLUGIN_DIGEST = "a".repeat(64);

export const VISUAL_PRIMARY_MODEL_CONTEXT_WINDOW = 1_050_000;
export const VISUAL_CONTEXT_TOKENS = 96_000;

export const VISUAL_NOW = Date.parse("2026-07-31T14:30:00Z");

export const VISUAL_RUNTIME_FEATURES = [
  "git",
  "plan",
  "skills",
  "knowledge",
  "agentMemory",
  "schedules",
  "relocate",
] as const;

export function visualFeatureCapabilities(): Record<
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

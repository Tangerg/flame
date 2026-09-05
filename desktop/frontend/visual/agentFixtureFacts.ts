/** Stable cross-layer facts owned by the canonical agent visual fixture. */
export const VISUAL_PRIMARY_MODEL_CONTEXT_WINDOW = 1_050_000;
export const VISUAL_CONTEXT_TOKENS = 96_000;

/**
 * The instant every fixture is photographed at.
 *
 * The harness advances `Date.now` from here by the page's real age, because scroll and
 * disclosure libraries drive their frame loops off it and a constant clock leaves them
 * waiting forever. That age is the one thing about a frame that load can change, so a spec
 * freezing "whatever the clock says now" freezes it INTO the golden: a running turn's
 * elapsed label read `390m 0s` or `390m 1s` depending on how long bootstrap took, and the
 * frame differed by a whole line of text. Freezing to this instead makes it exactly 390m.
 */
export const VISUAL_NOW = Date.parse("2026-07-31T14:30:00Z");

/**
 * The Runtime every fixture connects to, and the one place its features are listed.
 *
 * A feature the fixture omits is not neutral: the surface gated on it renders its off-ramp
 * and its real content is never drawn. That cost `skills`, `knowledge` and `agentMemory`
 * once; when it was paid, `schedules` and `relocate` were still missing, so the whole
 * schedules pane read "Schedules unavailable" and the cwd banner's relocate action — its
 * editor, its apply, its cancel — had never existed in a golden. `capabilityGate.test.ts`
 * now derives the required list from the app's own `useRuntimeCapability` call sites.
 */
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

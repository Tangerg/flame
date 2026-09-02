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

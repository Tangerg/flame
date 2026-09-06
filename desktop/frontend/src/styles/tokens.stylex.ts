import * as stylex from "@stylexjs/stylex";

/**
 * The design decisions, as the only values a StyleX prop will accept.
 *
 * Each one resolves to the CSS variable `globals.css` already owns rather than to a literal.
 * That is deliberate and load-bearing: those variables are computed — a shape step is
 * `--style-shape-md × --radius-scale × --corner-scale` — and they are redefined per theme and
 * per visual style. Inlining the value here would freeze one theme's answer into the token and
 * take the corner ladder, the density scale and every alternate style with it.
 *
 * So StyleX owns the VOCABULARY (what a component may name) and `globals.css` keeps owning the
 * RESOLUTION (what that name is worth right now, under this theme, at this scale).
 */

/** Ink, by role. A name says what the text IS, never how dark it is. */
export const color = stylex.defineVars({
  fg: "var(--color-text)",
  fgSoft: "var(--color-text-soft)",
  fgMuted: "var(--color-text-muted)",
  fgFaint: "var(--color-text-faint)",
  accent: "var(--color-accent)",
  onAccent: "var(--color-on-accent)",
  negative: "var(--color-negative)",
  warning: "var(--color-warning)",
  success: "var(--color-success)",
  info: "var(--color-info)",
});

/** How long a change takes. Durations are decisions; `150ms` at a call site is not. */
export const motion = stylex.defineVars({
  fast: "var(--dur-fast)",
  instant: "var(--dur-instant)",
  med: "var(--dur-med)",
  riseIn: "var(--animate-rise-in)",
  color: "var(--dur-color)",
  easeOut: "var(--ease-out)",
  /** Whole `animation` shorthands, so each duration keeps tracking motion-scale. */
  shimmer: "var(--animate-shimmer)",
  sweep: "var(--animate-sweep)",
  pulseDot: "var(--animate-pulse-dot)",
});

/** Surfaces and edges, by role. A name says what a plane IS, never how light it is. */
export const surface = stylex.defineVars({
  sunken: "var(--color-sunken)",
  surface2: "var(--color-surface-2)",
  divider: "var(--color-divider)",
  surface3: "var(--color-surface-3)",
  canvas: "var(--color-canvas)",
  floating: "var(--app-floating-surface)",
  scrim: "var(--color-scrim)",
  /** Row states are an ink wash whose strength tracks `--depth-step`, not a surface step. */
  hover: "var(--color-hover)",
  selected: "var(--color-selected)",
  negativeWashRow: "var(--color-negative-wash)",
  lineSoft: "var(--color-line-soft)",
  mediaField: "var(--color-media-preview)",
  /** The app's card plane, and the hairline a fill-less surface uses instead of it. */
  card: "var(--app-card-surface)",
  field: "var(--color-field)",
  fieldStrong: "var(--color-field-strong)",
  /** A tone wash, not the tone: 18% of the hue over whatever is behind it. */
  accentWash: "var(--color-accent-badge)",
  successWash: "var(--color-success-badge)",
  warningWash: "var(--color-warning-badge)",
  negativeWash: "var(--color-negative-badge)",
  infoWash: "var(--color-info-badge)",
});

/** Corner steps, each already carrying the style scale and the superellipse compensation. */
export const radius = stylex.defineVars({
  step2xs: "var(--radius-2xs)",
  xs: "var(--radius-xs)",
  /** Corners named for the plane they belong to: a card and a transcript bubble differ. */
  card: "var(--surface-card-radius)",
  bubble: "var(--radius-bubble)",
  sm: "var(--radius-sm)",
  /** Corners a control owns, which the visual style may move independently of the ladder. */
  field: "var(--field-radius)",
  segmented: "var(--segmented-radius)",
  segment: "var(--segment-radius)",
  floatingPanel: "var(--floating-panel-radius)",
  floatingTip: "var(--floating-tip-radius)",
  composer: "var(--radius-composer)",
  xl: "var(--radius-xl)",
});

/**
 * The 4px step, mirrored rather than renamed.
 *
 * These are numbers and a number is not a decision — the whole argument for moving to StyleX
 * says so. They stay numbers HERE on purpose: a styling-engine migration and a token-vocabulary
 * redesign done in one pass make every golden diff ambiguous, because nothing says whether a
 * frame moved because StyleX renders differently or because a spacing step changed value.
 * Mechanical first, with rendering held still; the roles come in their own pass, where each
 * frame that moves has exactly one cause.
 */
export const space = stylex.defineVars({
  s0_5: "calc(var(--spacing) * 0.5)",
  s1: "calc(var(--spacing) * 1)",
  s1_5: "calc(var(--spacing) * 1.5)",
  s2: "calc(var(--spacing) * 2)",
  s2_5: "calc(var(--spacing) * 2.5)",
  s3: "calc(var(--spacing) * 3)",
  s4: "calc(var(--spacing) * 4)",
  s4_5: "calc(var(--spacing) * 4.5)",
  s5: "calc(var(--spacing) * 5)",
  s6: "calc(var(--spacing) * 6)",
  s7: "calc(var(--spacing) * 7)",
  s8: "calc(var(--spacing) * 8)",
  s9: "calc(var(--spacing) * 9)",
  s10: "calc(var(--spacing) * 10)",
  s12: "calc(var(--spacing) * 12)",
  s24: "calc(var(--spacing) * 24)",
});

/**
 * A type STEP, not a font size.
 *
 * `text-ui-xs` was never one decision: Tailwind's type utilities carry a size and the
 * tracking that was chosen with it, and reading only the size out of the ladder is how the
 * first migrated component came out 2.1px wider than the one it replaced — the tracking was
 * gone and nothing said so, because `fontSize` alone is a legal, complete-looking style.
 *
 * So a step is a bundle here too, and a call site names the step rather than assembling one.
 */
/** How tall a line is. A ratio at a call site is not a decision; these are. */
export const leading = stylex.defineVars({
  body: "var(--leading-body)",
  relaxed: "var(--leading-relaxed)",
  prose: "var(--leading-prose)",
  snug: "var(--leading-snug)",
});

/**
 * A corner STEP, not a radius — the same lesson `type` learned, in the other ladder.
 *
 * Every corner in the product is a superellipse: `globals.css` sets `corner-shape:
 * superellipse(1.5)` on everything and compensates the radius by `--corner-scale`. The pill
 * step opts back out, because a superellipse at pill radius is a rounded square rather than a
 * circle — and that opt-out was keyed on Tailwind's own class names (`.rounded-full`,
 * `.rounded-pill`). A StyleX component never carries those, so setting the radius alone turned
 * every circle in the design system into a squircle: silently, because at a 6px dot or a 12px
 * ring the difference is sub-pixel, and only the 40px empty-state icon was large enough for a
 * golden to see it.
 *
 * So the pill is a bundle and `radius` no longer exposes it: the two halves cannot be separated
 * because they were never two decisions.
 */
export const corner = stylex.create({
  pill: { borderRadius: "var(--radius-pill)", "corner-shape": "round" },
});

export const type = stylex.create({
  ui2xs: { fontSize: "var(--text-ui-2xs)", letterSpacing: "var(--text-ui-2xs--letter-spacing)" },
  uiXs: { fontSize: "var(--text-ui-xs)", letterSpacing: "var(--text-ui-xs--letter-spacing)" },
  uiSm: { fontSize: "var(--text-ui-sm)", letterSpacing: "var(--text-ui-sm--letter-spacing)" },
  uiMd: { fontSize: "var(--text-ui-md)", letterSpacing: "var(--text-ui-md--letter-spacing)" },
  displaySm: {
    fontSize: "var(--text-display-sm)",
    letterSpacing: "var(--text-display-sm--letter-spacing)",
  },
  code: { fontSize: "var(--text-code)", letterSpacing: "var(--text-code--letter-spacing)" },
  prose: { fontSize: "var(--text-prose)", letterSpacing: "var(--text-prose--letter-spacing)" },
});

/**
 * The two faces, as the pair of decisions each one actually is.
 *
 * A face is never only a family. The UI steps carry `--tracking-ui`, a negative tracking chosen
 * for a proportional face; mono glyphs are already spaced by the grid and take that tracking as
 * a crowding defect. Every call site that reached for `font-mono` got the family and kept the
 * tracking, because tracking is not in that utility — which is exactly why this is a token and
 * not two properties a component sets side by side.
 */
export const face = stylex.create({
  text: { fontFamily: "var(--font-sans)" },
  mono: { fontFamily: "var(--font-mono)", letterSpacing: 0 },
});

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
  onAccent: "var(--color-text-on-accent)",
  /** The ink that reads on the CTA fill. The fill itself is a surface, not an ink. */
  ctaText: "var(--color-cta-text)",
  onMedia: "var(--color-on-media)",
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
  spin: "var(--animate-spin)",
  breathe: "var(--animate-breathe)",
});

/** Surfaces and edges, by role. A name says what a plane IS, never how light it is. */
export const surface = stylex.defineVars({
  sunken: "var(--color-sunken)",
  sunkenHover: "var(--color-sunken-hover)",
  surface2: "var(--color-surface-2)",
  divider: "var(--color-divider)",
  surface3: "var(--color-surface-3)",
  surface: "var(--color-surface)",
  canvas: "var(--color-bg)",
  floating: "var(--app-floating-surface)",
  scrim: "var(--color-scrim)",
  /** Row states are an ink wash whose strength tracks `--depth-step`, not a surface step. */
  hover: "var(--wash-hover)",
  ctaFill: "var(--color-cta)",
  ctaHover: "var(--color-cta-hover)",
  mediaScrim: "var(--color-media-scrim)",
  /** A row's wash: 10% of the hue, enough to tint without becoming a plate. */
  accentWash: "var(--color-accent-wash)",
  negativeWash: "var(--color-negative-wash)",
  infoWash: "var(--color-info-wash)",
  successWash: "var(--color-success-wash)",
  warningWash: "var(--color-warning-wash)",
  joinSeam: "var(--button-join-seam)",
  selected: "var(--wash-selected)",
  selectedHover: "var(--wash-selected-hover)",
  lineSoft: "var(--color-line-soft)",
  mediaField: "var(--color-media-preview)",
  /** The app's card plane, and the hairline a fill-less surface uses instead of it. */
  card: "var(--app-card-surface)",
  field: "var(--color-border)",
  fieldStrong: "var(--color-border-soft)",
  /** A badge's wash: 18% of the hue over whatever is behind it. Stronger than the row wash
   *  above, because a badge has to hold its own shape rather than tint a row. */
  accentBadge: "var(--color-accent-badge)",
  successBadge: "var(--color-success-badge)",
  warningBadge: "var(--color-warning-badge)",
  negativeBadge: "var(--color-negative-badge)",
  infoBadge: "var(--color-info-badge)",
});

/** Corner steps, each already carrying the style scale and the superellipse compensation. */
export const radius = stylex.defineVars({
  step2xs: "var(--shape-2xs)",
  xs: "var(--shape-xs)",
  /** Corners named for the plane they belong to: a card and a transcript bubble differ. */
  card: "var(--surface-card-radius)",
  /** The transcript column's corner. Shared by everything that sits in it — the reader's own
   *  message and the cards the agent puts beside it — because they are the same width apart
   *  from the same edge. What is NOT shared is the shape: see `corner.bubble`. */
  bubble: "var(--shape-bubble)",
  sm: "var(--shape-sm)",
  lg: "var(--shape-lg)",
  /** Corners a control owns, which the visual style may move independently of the ladder. */
  field: "var(--field-radius)",
  segmented: "var(--segmented-radius)",
  segment: "var(--segment-radius)",
  row: "var(--row-radius)",
  button: "var(--button-radius)",
  floatingPanel: "var(--floating-panel-radius)",
  floatingTip: "var(--floating-tip-radius)",
  composer: "var(--shape-composer)",
  xl: "var(--shape-xl)",
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
  s3_5: "calc(var(--spacing) * 3.5)",
  s4: "calc(var(--spacing) * 4)",
  s4_5: "calc(var(--spacing) * 4.5)",
  s5: "calc(var(--spacing) * 5)",
  s6: "calc(var(--spacing) * 6)",
  s7: "calc(var(--spacing) * 7)",
  s8: "calc(var(--spacing) * 8)",
  s9: "calc(var(--spacing) * 9)",
  s10: "calc(var(--spacing) * 10)",
  s11: "calc(var(--spacing) * 11)",
  s12: "calc(var(--spacing) * 12)",
  s14: "calc(var(--spacing) * 14)",
  s16: "calc(var(--spacing) * 16)",
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
/**
 * How heavy a face is. `regular` is 430 rather than 400 — the variable face is set a notch up so
 * body text holds its colour on a dark ground — which is exactly why these are tokens: a literal
 * `400` here reads as correct and renders a step light.
 */
export const weight = stylex.defineVars({
  regular: "var(--fw-regular)",
  medium: "var(--fw-medium)",
  semibold: "var(--fw-semibold)",
});

export const leading = stylex.defineVars({
  body: "var(--leading-body)",
  relaxed: "var(--leading-relaxed)",
  prose: "var(--leading-prose)",
  snug: "var(--leading-snug)",
  tight: "var(--leading-tight)",
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
  pill: { borderRadius: "var(--shape-pill)", "corner-shape": "round" },
  /**
   * SPEECH, and a bundle for the same reason the pill is one: a squircle is the shape of
   * chrome, so a block someone typed drawn with one reads as another panel.
   *
   * Only the reader's own message. An approval or a question card sits in the same column and
   * takes the same RADIUS (`radius.bubble`), but it carries controls and is answered rather
   * than read — it is chrome that arrived in the transcript, and it keeps the squircle. The
   * reference client splits it the same way: its two message-bubble components each override
   * the squircle back to round, while the cards in its thread keep it.
   */
  bubble: { borderRadius: "var(--shape-bubble)", "corner-shape": "round" },
});

export const type = stylex.create({
  ui2xs: { fontSize: "var(--fs-ui-2xs)", letterSpacing: "var(--text-ui-2xs--letter-spacing)" },
  uiXs: { fontSize: "var(--fs-ui-xs)", letterSpacing: "var(--tracking-ui)" },
  uiSm: { fontSize: "var(--fs-ui-sm)", letterSpacing: "var(--tracking-ui)" },
  uiMd: { fontSize: "var(--fs-ui-md)", letterSpacing: "var(--tracking-ui)" },
  // A display step carries THREE halves, not one: `md` and `lg` bring their own leading
  // because a heading's line box is tighter than the body's. Six call sites had copied only
  // the size, which at the largest font size left a 26px heading on the transcript's leading.
  displaySm: {
    fontSize: "var(--fs-display-sm)",
    letterSpacing: "var(--tracking-ui)",
  },
  displayMd: {
    fontSize: "var(--fs-display-md)",
    letterSpacing: "var(--tracking-display)",
    lineHeight: "var(--text-display-md--line-height)",
  },
  displayLg: {
    fontSize: "var(--fs-display-lg)",
    letterSpacing: "var(--tracking-display)",
    lineHeight: "var(--text-display-lg--line-height)",
  },
  code: { fontSize: "var(--fs-code)", letterSpacing: "var(--text-code--letter-spacing)" },
  prose: { fontSize: "var(--fs-prose)", letterSpacing: "var(--tracking-ui)" },
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

import * as stylex from "@stylexjs/stylex";
import { color, space, weight } from "@/styles/tokens.stylex";

/**
 * The pieces every surface is arranged from.
 *
 * A built-in context may not import another's internals, so the only place one copy can live
 * is here.
 *
 * What belongs in this file: a fact with exactly one sensible value that every surface needs.
 * What does not: an arrangement a surface DECIDES — a row's gap, a card's inset — which stays
 * with that surface, under a name that says which surface it is.
 */
export const vocab = stylex.create({
  fill: { minWidth: 0, flex: 1 },
  grow: { flex: 1 },
  hold: { flexShrink: 0 },
  min: { minWidth: 0 },
  column: { display: "flex", flexDirection: "column" },
  line: { display: "flex", alignItems: "center", gap: space.s2 },
  lineTight: { display: "flex", alignItems: "center", gap: space.s1_5 },

  afterLine: { marginTop: space.s1_5 },

  truncate: { overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" },
  wrapText: { whiteSpace: "pre-wrap", overflowWrap: "break-word" },
  pretty: { textWrap: "pretty" },
  figures: { fontVariantNumeric: "tabular-nums" },
  strong: { fontWeight: weight.semibold },

  // The ink ladder as styles. `toneInk` maps the domain's `Tone` onto these same steps.
  ink: { color: color.fg },
  soft: { color: color.fgSoft },
  muted: { color: color.fgMuted },
  faint: { color: color.fgFaint },
  accent: { color: color.accent },
  success: { color: color.success },
  warning: { color: color.warning },
  negative: { color: color.negative },
  info: { color: color.info },
});

/**
 * A stack's rhythm, as a ladder of rungs.
 *
 * Composes with `vocab.column`, which declares NO gap. Never put one of these after a step
 * that brings its own — `vocab.line` is gap-2 by default, and two steps claiming `gap` is how
 * an override becomes invisible.
 */
export const gap = stylex.create({
  s0_5: { gap: space.s0_5 },
  s1: { gap: space.s1 },
  s1_5: { gap: space.s1_5 },
  s2: { gap: space.s2 },
  s2_5: { gap: space.s2_5 },
  s3: { gap: space.s3 },
  s4: { gap: space.s4 },
  s6: { gap: space.s6 },
});

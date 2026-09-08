import * as stylex from "@stylexjs/stylex";
import { color, space, weight } from "@/styles/tokens.stylex";

/**
 * The pieces every surface is arranged from.
 *
 * These lived in FOUR copies — one per plugin — because a built-in context may not import
 * another's internals, so the dock, the settings panes, the transcript and the shell each grew
 * their own `truncate`, `min`, `hold`, `muted`. Nineteen names, identical values, four owners.
 * The plugin boundary is real, so the only place one copy can live is here.
 *
 * What belongs in this file: a fact with exactly one sensible value that every surface needs.
 * What does not: an arrangement a surface DECIDES — a row's gap, a card's inset — which stays
 * with that surface, under a name that says which surface it is.
 */
export const vocab = stylex.create({
  /** A part that gives up its width so a trailing chip keeps its own. */
  fill: { minWidth: 0, flex: 1 },
  grow: { flex: 1 },
  /** A part that keeps its width while everything beside it yields. */
  hold: { flexShrink: 0 },
  min: { minWidth: 0 },
  column: { display: "flex", flexDirection: "column" },
  /** A row of things read left to right. A surface that wants a different gap says so. */
  line: { display: "flex", alignItems: "center", gap: space.s2 },
  lineTight: { display: "flex", alignItems: "center", gap: space.s1_5 },
  stackHairline: { display: "flex", flexDirection: "column", gap: space.s0_5 },

  truncate: { overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" },
  /** Text that keeps the breaks it was written with, and breaks words if it must. */
  wrapText: { whiteSpace: "pre-wrap", overflowWrap: "break-word" },
  pretty: { textWrap: "pretty" },
  /** Numbers that change in place: one advance width, so a column cannot jitter. */
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
});

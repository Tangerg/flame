import * as stylex from "@stylexjs/stylex";
import { color, leading, radius, space, surface, weight } from "@/styles/tokens.stylex";

/**
 * The shapes a settings pane is made of.
 *
 * Twenty-nine files render the same handful — a stack of groups, a row that splits a label
 * from its control, a label, a hint under it — and each had written them out: the label eight
 * times, the hint seven in two spellings, the stack eleven at two gaps.
 *
 * Type steps are not here. A size is the design system's vocabulary and a pane composes it;
 * this file owns only the arrangement, the same split `viewStyles` makes for the dock.
 */
export const settingStyles = stylex.create({
  /** A pane is a column of groups. The wider gap separates GROUPS, the narrower one rows. */
  pane: { display: "flex", flexDirection: "column", gap: space.s6 },
  stack: { display: "flex", flexDirection: "column", gap: space.s3 },
  stackTight: { display: "flex", flexDirection: "column", gap: space.s2 },

  /** A label and whatever answers it, held apart. */
  split: { display: "flex", alignItems: "center", justifyContent: "space-between", gap: space.s3 },
  line: { display: "flex", alignItems: "center", gap: space.s2 },
  lineTight: { display: "flex", alignItems: "center", gap: space.s1_5 },
  lineWide: { display: "flex", alignItems: "center", gap: space.s3 },
  lineWrap: { display: "flex", flexWrap: "wrap", alignItems: "center", gap: space.s1_5 },
  end: { display: "flex", justifyContent: "flex-end" },

  label: { color: color.fg, fontWeight: weight.medium },
  /** What a setting DOES, under its name. `snug` because it is one or two lines, not prose. */
  hint: { color: color.fgMuted, lineHeight: leading.snug },
  /** The same hint where it needs to clear the control above it. */
  hintSpaced: { marginTop: space.s1, color: color.fgMuted, lineHeight: leading.body },
  intro: { color: color.fgMuted, lineHeight: leading.body },

  fill: { minWidth: 0, flex: 1 },
  hold: { flexShrink: 0 },
  min: { minWidth: 0 },
  truncate: { overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" },
  mono: { fontFamily: "var(--font-mono)" },
  muted: { color: color.fgMuted },
  faint: { color: color.fgFaint },
  accent: { color: color.accent },
  warning: { color: color.warning },
  negative: { color: color.negative },

  /** A row the pointer can act on, inside a group that is already a card. */
  sunkenRow: {
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
    gap: space.s3,
    borderRadius: radius.card,
    backgroundColor: { default: surface.sunken, ":hover": surface.hover },
    paddingInline: space.s3,
    paddingBlock: space.s1_5,
    transitionProperty: "background-color",
  },
});

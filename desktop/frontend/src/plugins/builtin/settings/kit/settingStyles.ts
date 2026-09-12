import * as stylex from "@stylexjs/stylex";
import { color, leading, motion, radius, space, surface, weight } from "@/styles/tokens.stylex";

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
  stack: { display: "flex", flexDirection: "column", gap: space.s3 },

  split: { display: "flex", alignItems: "center", justifyContent: "space-between", gap: space.s3 },
  lineWide: { display: "flex", alignItems: "center", gap: space.s3 },
  lineWrap: { display: "flex", flexWrap: "wrap", alignItems: "center", gap: space.s1_5 },
  end: { display: "flex", justifyContent: "flex-end" },

  label: { color: color.fg, fontWeight: weight.medium },
  /** What a setting DOES, under its name. `snug` because it is one or two lines, not prose. */
  hint: { color: color.fgMuted, lineHeight: leading.snug },
  /** The same hint where it needs to clear the control above it. */
  hintSpaced: { marginTop: space.s1, color: color.fgMuted, lineHeight: leading.body },
  intro: { color: color.fgMuted, lineHeight: leading.body },

  hoverRow: {
    borderRadius: radius.card,
    backgroundColor: { default: null, ":hover": surface.hover },
    paddingInline: space.s3,
    paddingBlock: space.s2_5,
    transitionProperty: "background-color",
  },
  hoverRowTall: { paddingBlock: space.s3 },
  hoverRowTight: { paddingInline: space.s2, paddingBlock: space.s2 },
  /** A name that gives up its width, beside a control that keeps its own. */
  nameGrid: {
    display: "grid",
    gridTemplateColumns: "minmax(0, 1fr) auto",
    alignItems: "center",
    gap: space.s3,
  },
  monoName: {
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
    fontFamily: "var(--font-mono)",
    color: color.fg,
  },
  /** A group's own caption, above the rows rather than beside one. */
  caption: { marginBottom: space.s1_5, color: color.fgMuted, fontWeight: weight.medium },
  inline: { display: "inline-flex", alignItems: "center", gap: space.s1 },
  grid2: { display: "grid", gap: space.s2 },
  selfEnd: { alignSelf: "flex-end" },
  /** A caption on the same line as what it labels, rather than above a group. */
  captionInline: { color: color.fgMuted, fontWeight: weight.medium },
  spin: { animation: motion.spin },
  afterRow: { marginTop: space.s2_5 },

  sunkenRow: {
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
    gap: space.s3,
    borderRadius: radius.card,
    backgroundColor: { default: surface.sunken, ":hover": surface.sunkenHover },
    paddingInline: space.s3,
    paddingBlock: space.s1_5,
    transitionProperty: "background-color",
  },
});

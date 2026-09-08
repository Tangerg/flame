import * as stylex from "@stylexjs/stylex";
import { color, leading, space, surface, weight } from "@/styles/tokens.stylex";

/**
 * The shapes a dock view is made of.
 *
 * Twenty-three views render the same thing — a column of rows, each with a title line, a
 * description under it and a caption beside it — and each had written that out. The gutter
 * alone appeared thirty-six times. These are the pieces they agreed on, named once, so a
 * view says what it is showing rather than how wide its own inset is.
 *
 * Type steps are NOT here: a view composes `typeStep.uiMd` and friends from the design
 * system, because a size is the design's vocabulary and this file is only the arrangement.
 */
export const viewStyles = stylex.create({
  /** The dock's own horizontal inset. Every view shares it; see `--density-column-gutter-wide`. */
  gutter: { paddingInline: "var(--density-column-gutter-wide)" },
  stack: { display: "flex", flexDirection: "column" },

  rowPad: { paddingBlock: space.s2 },
  rowPadTall: { paddingBlock: space.s2_5 },
  padBlockSm: { paddingBlock: space.s1 },
  stackGap: { gap: space.s4 },
  /** A group's heading sits closer to its own rows than to the group above it. */
  sectionPad: { paddingBottom: space.s1 },
  sectionLabel: { paddingInline: space.s2, paddingBlock: space.s2 },
  planPad: { paddingBlock: space.s3_5 },
  planHeading: { paddingInline: 0, paddingTop: 0, paddingBottom: space.s2 },

  /** A row whose whole body is the button. `wash` is the row state the design system gives it. */
  pressRow: {
    display: "flex",
    width: "100%",
    minWidth: 0,
    alignItems: "flex-start",
    gap: space.s2_5,
    textAlign: "left",
  },
  rowTop: { display: "flex", alignItems: "flex-start", gap: space.s2_5 },
  wash: {
    transitionProperty: "background-color",
    backgroundColor: { default: null, ":hover": surface.hover },
  },
  /** A glyph beside a first line sits on the line's baseline, not the box's top edge. */
  glyphTop: { marginTop: space.s0_5 },
  /** A glyph on a baseline row centres itself instead: it has no baseline of its own. */
  glyphInline: { flexShrink: 0, alignSelf: "center", color: color.fgMuted },
  grow: { flex: 1 },
  figures: { fontVariantNumeric: "tabular-nums" },
  /** Where a row's own trailing text sits, one step below the description above it. */
  origin: { marginTop: space.s1, color: color.fgFaint },
  actions: { display: "flex", flexShrink: 0, alignItems: "center", gap: space.s2 },
  afterRow: { marginTop: space.s1_5 },
  meterLine: { marginTop: space.s1, display: "flex", alignItems: "center", gap: space.s2_5 },
  dotTop: { marginTop: space.s1_5 },
  subLine: {
    marginTop: space.s0_5,
    display: "flex",
    minWidth: 0,
    alignItems: "center",
    gap: space.s1_5,
  },
  /** A dismissed notice is still readable — it has been dealt with, not disabled. */
  dismissed: { opacity: 0.5 },

  /** A row's first line: what it is, plus whatever sits beside the name. */
  line: { display: "flex", alignItems: "center", gap: space.s2, minWidth: 0 },
  lineBaseline: { display: "flex", alignItems: "baseline", gap: space.s2, minWidth: 0 },
  lineTop: { display: "flex", alignItems: "flex-start", gap: space.s3, minWidth: 0 },
  /** A name that gives up its width beside something that keeps its own. */
  splitLine: {
    display: "grid",
    gridTemplateColumns: "minmax(0, 1fr) auto",
    alignItems: "baseline",
    gap: space.s2,
  },

  /** The part of a row that gives up its width so a trailing chip keeps its own. */
  fill: { minWidth: 0, flex: 1 },
  hold: { flexShrink: 0 },
  pushEnd: { marginInlineStart: "auto" },
  truncate: { overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" },

  title: { color: color.fg, fontWeight: weight.semibold },
  /** The line under a title. `leading.body` because it wraps and a title's leading does not. */
  description: { marginTop: space.s0_5, color: color.fgMuted, lineHeight: leading.body },
  caption: { color: color.fgFaint },
  /** A caption directly under a title, which owns the gap between them. */
  subCaption: { marginTop: space.s0_5, color: color.fgFaint },
  min: { minWidth: 0 },
  subCaptionMuted: { marginTop: space.s0_5, color: color.fgMuted },
  warning: { color: color.warning },
  wrapText: { whiteSpace: "pre-wrap", overflowWrap: "break-word" },
  soft: { color: color.fgSoft },
  muted: { color: color.fgMuted },
  mono: { fontFamily: "var(--font-mono)" },
});

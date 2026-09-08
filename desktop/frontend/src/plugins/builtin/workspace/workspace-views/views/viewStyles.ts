import * as stylex from "@stylexjs/stylex";
import { color, leading, radius, space, surface, weight } from "@/styles/tokens.stylex";

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
  actionsTight: { display: "flex", flexShrink: 0, alignItems: "center", gap: space.s0_5 },
  body: { color: color.fg, lineHeight: leading.body },
  metaLine: { marginTop: space.s1, display: "flex", alignItems: "center", gap: space.s2 },
  formLine: { marginTop: space.s2, display: "flex", alignItems: "center", gap: space.s2 },
  filterLine: { display: "flex", alignItems: "center", gap: space.s1 },
  pinLine: { display: "flex", flexShrink: 0, alignItems: "center", gap: space.s1 },
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
  /** Dealt with, still listed. Not disabled — the row is telling you it happened. */
  dismissed: { opacity: "var(--state-receded)" },

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
  /** The interpunct between two facts in a header: a glyph-only box, so no leading. */
  dotSep: { lineHeight: 1, color: color.fgFaint },
  subCaptionMuted: { marginTop: space.s0_5, color: color.fgMuted },
  warning: { color: color.warning },
  wrapText: { whiteSpace: "pre-wrap", overflowWrap: "break-word" },
  soft: { color: color.fgSoft },
  muted: { color: color.fgMuted },
  negative: { color: color.negative },
  accent: { color: color.accent },
  success: { color: color.success },
  info: { color: color.info },
  semibold: { fontWeight: weight.semibold },

  /** A section's heading line, and the rhythm between it and the entries under it. */
  sectionHead: { marginBottom: space.s1_5, display: "flex", alignItems: "baseline", gap: space.s2 },
  sectionBody: { display: "grid", gap: space.s1 },
  sectionOuterPad: { paddingBlock: space.s3 },
  /** One entry in a section: a glyph, the thing, and whatever the thing scored. */
  entry: {
    display: "flex",
    alignItems: "baseline",
    gap: space.s2,
    fontFamily: "var(--font-mono)",
  },
  entryPlain: { display: "flex", alignItems: "baseline", gap: space.s2 },
  statusPad: { paddingTop: space.s1, paddingBottom: space.s2 },
  ink: { color: color.fg },
  padBottom: { paddingBottom: space.s2 },
  groupPad: { paddingBlock: space.s1_5 },
  afterTitle: { marginTop: space.s0_5 },
  /** The count beside a path reads as a quantity, not as part of the name. */
  matchCount: { marginInlineStart: space.s1_5, fontWeight: weight.regular, color: color.fgFaint },
  /** A match: its line number in a fixed column so the text of every match starts level. */
  matchRow: {
    display: "grid",
    width: "100%",
    gridTemplateColumns: "calc(var(--spacing) * 11) minmax(0, 1fr)",
    gap: space.s2,
    borderRadius: radius.xs,
    paddingBlock: "1px",
    paddingRight: space.s1,
    fontFamily: "var(--font-mono)",
    lineHeight: leading.body,
  },
  lineNumber: { textAlign: "right", color: color.fgFaint, userSelect: "none" },
  /** A row that opens something: a chevron, the name, and what the name belongs to. */
  discloseRow: {
    display: "grid",
    gridTemplateColumns: "calc(var(--spacing) * 3.5) minmax(0, 1fr) auto",
    alignItems: "center",
    gap: space.s2,
    borderWidth: 0,
    backgroundColor: "transparent",
    textAlign: "left",
  },
  chevron: { color: color.fgFaint, transitionProperty: "rotate" },
  chevronShut: { rotate: "-90deg" },
  editorGap: { gap: space.s2 },
  /** The panel indents past the chevron so its content lines up with the name above it. */
  editorInset: { paddingBottom: space.s3, paddingLeft: space.s10 },
});

/**
 * The timeline is the one view whose rows nest: a delegated run is drawn inside its parent.
 * The indent is a ladder rather than `depth × step` because its last rung is a cap — past
 * five levels a further indent buys nothing and costs the text its width.
 */
export const timelineStyles = stylex.create({
  glyph: { marginTop: space.s1, flexShrink: 0, color: color.fgFaint },
  kind: { color: color.fg, fontWeight: weight.medium },
  // The box holds a glyph and no text, so it takes no leading: a type step here would add
  // descender space under a check mark and push the row taller than the line beside it.
  mark: { marginTop: space.s1, flexShrink: 0, lineHeight: 1 },
  stamp: {
    marginTop: space.s0_5,
    flexShrink: 0,
    fontFamily: "var(--font-mono)",
    color: color.fgFaint,
  },
  runHeader: {
    display: "flex",
    minHeight: space.s10,
    alignItems: "center",
    gap: space.s2,
    // A run header is a plate, and a plate takes the corner its own plane owns rather than a
    // rung of the ladder — `--surface-card-radius` and `--radius-md` are the same value under
    // two names, and only one of them is the one a visual style may move.
    borderRadius: radius.card,
    backgroundColor: surface.sunken,
    paddingLeft: space.s3,
  },
  runDetail: {
    marginTop: space.s0_5,
    display: "flex",
    minWidth: 0,
    gap: space.s2,
    color: color.fgMuted,
  },
  pretty: { textWrap: "pretty" },
  groupGap: { marginTop: space.s3, paddingTop: space.s1 },
  nested: {
    borderLeftWidth: "1px",
    borderLeftStyle: "solid",
    borderLeftColor: surface.field,
    paddingLeft: space.s2,
  },
});

const indentStyles = stylex.create({
  d0: {},
  d1: { marginLeft: space.s3 },
  d2: { marginLeft: space.s6 },
  d3: { marginLeft: space.s9 },
  d4: { marginLeft: space.s12 },
  d5: { marginLeft: space.s16 },
});

export const indent = [
  indentStyles.d0,
  indentStyles.d1,
  indentStyles.d2,
  indentStyles.d3,
  indentStyles.d4,
  indentStyles.d5,
] as const;

/** The tools view: a catalogue whose rows open a panel that can invoke what the row names. */
export const toolStyles = stylex.create({
  headPad: { paddingTop: space.s2, paddingBottom: space.s1 },
  familyList: { paddingBottom: space.s1_5 },
  blurb: { paddingBottom: space.s2, color: color.fgMuted, lineHeight: leading.body },
  toolRow: {
    display: "grid",
    gridTemplateColumns: "calc(var(--spacing) * 3.5) auto minmax(0, 1fr)",
    alignItems: "flex-start",
    gap: space.s2_5,
    paddingBlock: space.s1,
    textAlign: "left",
  },
  /** Both glyphs sit on the first line of a two-line cell, not on the cell's top edge. */
  rowGlyph: { marginTop: space.s1 },
  toolBlurb: { display: "block", color: color.fgFaint },
  panelGap: { gap: space.s2_5 },
  /** The panel starts where the row's NAME does, past both glyphs and their gaps. */
  panelInset: { paddingTop: space.s1, paddingBottom: space.s3, paddingLeft: "58px" },
  fieldGap: { gap: space.s1 },
  fieldLabel: { color: color.fgMuted, fontWeight: weight.medium },
  afterLabel: { marginTop: space.s1 },
  footer: { paddingTop: space.s3_5, paddingBottom: "18px", lineHeight: leading.body },
});

/**
 * The code surfaces: a command log, a file, a diff.
 *
 * Their inset is NOT the dock gutter — it sits beside a line-number column and belongs to the
 * code, which is why `px-3` here was left alone when the panels moved to the density gutter.
 */
export const codeStyles = stylex.create({
  sheet: { paddingBlock: space.s2, fontFamily: "var(--font-mono)", lineHeight: leading.relaxed },
  /** A log rather than a file: it stacks entries, so it brings a gap and its own inset. Its
   *  block inset differs from `sheet`'s, which is why it is composed AFTER it, never before. */
  sheetInset: {
    display: "flex",
    flexDirection: "column",
    gap: space.s2_5,
    paddingInline: space.s3,
    paddingBlock: space.s3,
  },
  /** A gutter number is not read, it is counted against — so it never takes the selection. */
  gutter: { textAlign: "right", color: color.fgFaint, userSelect: "none" },
  /** Machine output wraps rather than scrolling: a long line is still one line of meaning. */
  wrap: { minWidth: 0, whiteSpace: "pre-wrap", overflowWrap: "anywhere" },
  soft: { color: color.fgSoft },
  hunk: {
    marginTop: space.s2_5,
    borderWidth: 0,
    backgroundColor: surface.sunken,
    paddingInline: space.s3,
    paddingBlock: space.s1,
    color: color.fgFaint,
  },
  split: { display: "grid", gridTemplateColumns: "repeat(2, minmax(0, 1fr))" },
  blank: { backgroundColor: surface.sunken },
  prompt: { flexShrink: 0, color: color.fgFaint },
  running: { flexShrink: 0, color: color.accent },
  failed: { flexShrink: 0, color: color.negative },
  output: {
    marginTop: space.s1_5,
    whiteSpace: "pre-wrap",
    overflowWrap: "break-word",
    color: color.fgMuted,
  },
  /** One file's diff, boxed: the border is the seam between two files' worth of lines. */
  fileCard: {
    marginBottom: space.s2,
    marginTop: { default: null, ":first-child": space.s2 },
    overflow: "hidden",
    borderRadius: radius.card,
    borderWidth: "0.5px",
    borderStyle: "solid",
    borderColor: surface.field,
  },
  fileCardLast: { marginBottom: 0 },
  note: {
    margin: 0,
    paddingInline: space.s3,
    paddingBlock: space.s2,
    fontFamily: "var(--font-mono)",
    color: color.fgFaint,
  },
});

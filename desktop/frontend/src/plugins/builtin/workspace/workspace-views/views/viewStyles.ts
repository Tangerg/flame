import * as stylex from "@stylexjs/stylex";
import { color, leading, motion, radius, space, surface, weight } from "@/styles/tokens.stylex";

/**
 * The shapes a dock view is made of.
 *
 * Type steps are NOT here: a view composes `typeStep.uiMd` and friends from the design
 * system, because a size is the design's vocabulary and this file is only the arrangement.
 */
export const viewStyles = stylex.create({
  gutter: { paddingInline: "var(--density-column-gutter-wide)" },

  rowPad: { paddingBlock: space.s2 },
  rowPadTall: { paddingBlock: space.s2_5 },
  padBlockSm: { paddingBlock: space.s1 },
  sectionPad: { paddingBottom: space.s1 },
  sectionLabel: { paddingInline: space.s2, paddingBlock: space.s2 },
  planPad: { paddingBlock: space.s3_5 },
  planHeading: { paddingInline: 0, paddingTop: 0, paddingBottom: space.s2 },

  /** The height, the inset and the type step stay with each tree. */
  treeRow: {
    display: "flex",
    width: "100%",
    minWidth: 0,
    alignItems: "center",
    gap: space.s1_5,
    borderRadius: radius.card,
    borderWidth: 0,
    backgroundColor: {
      default: "transparent",
      ":hover": surface.hover,
      ":focus-visible": surface.hover,
    },
    textAlign: "left",
    color: color.fg,
    transitionProperty: "background-color",
    transitionDuration: motion.color,
  },
  treeRowSelected: { backgroundColor: surface.selected },
  treeRowTall: { height: "calc(var(--spacing) * 7)", paddingRight: space.s2 },
  treeRowInset: { paddingInline: space.s1_5, paddingBlock: space.s1 },

  diffFileHeader: {
    display: "flex",
    height: space.s8,
    width: "100%",
    minWidth: 0,
    alignItems: "center",
    gap: space.s2,
    borderWidth: 0,
    backgroundColor: surface.sunken,
    paddingInline: space.s3,
    textAlign: "left",
    color: { default: color.fgMuted, ":hover": color.fg },
    transitionProperty: "color",
    transitionDuration: motion.color,
  },

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
  glyphTop: { marginTop: space.s0_5 },
  glyphInline: { flexShrink: 0, alignSelf: "center", color: color.fgMuted },
  origin: { marginTop: space.s1, color: color.fgFaint },
  actions: { display: "flex", flexShrink: 0, alignItems: "center", gap: space.s2 },
  actionsTight: { display: "flex", flexShrink: 0, alignItems: "center", gap: space.s0_5 },
  // What the agent wrote, so it can contain a path, a hash or an identifier with nowhere to
  // break. Without this the run paints straight out of the pane; `unbreakableContent` measured
  // 1636px of it on a memory entry.
  body: { color: color.fg, lineHeight: leading.body, overflowWrap: "break-word" },
  metaLine: { marginTop: space.s1, display: "flex", alignItems: "center", gap: space.s2 },
  formLine: { marginTop: space.s2, display: "flex", alignItems: "center", gap: space.s2 },
  filterLine: { display: "flex", alignItems: "center", gap: space.s1 },
  pinLine: { display: "flex", flexShrink: 0, alignItems: "center", gap: space.s1 },
  meterLine: { marginTop: space.s1, display: "flex", alignItems: "center", gap: space.s2_5 },
  dotTop: { marginTop: space.s1_5 },
  subLine: {
    marginTop: space.s0_5,
    display: "flex",
    minWidth: 0,
    alignItems: "center",
    gap: space.s1_5,
  },
  /** Not disabled — the row is telling you it happened. */
  dismissed: { opacity: "var(--state-receded)" },

  lineBaseline: { display: "flex", alignItems: "baseline", gap: space.s2, minWidth: 0 },
  lineTop: { display: "flex", alignItems: "flex-start", gap: space.s3, minWidth: 0 },
  splitLine: {
    display: "grid",
    gridTemplateColumns: "minmax(0, 1fr) auto",
    alignItems: "baseline",
    gap: space.s2,
  },
  /**
   * The name truncates, which gives a flex item an automatic minimum of zero, while a Tag or a
   * Badge keeps its width. So the one thing in the row that identifies it was the only thing
   * allowed to vanish: measured in a 207px dock, `review-diff` rendered at 0px wide beside a
   * revision hash and two badges that were all fully drawn — not ellipsed, absent.
   */
  titleLine: {
    display: "flex",
    flexWrap: "wrap",
    minWidth: 0,
    alignItems: "center",
    gap: space.s2,
  },

  pushEnd: { marginInlineStart: "auto" },

  title: { color: color.fg, fontWeight: weight.semibold },
  titleMedium: { color: color.fg, fontWeight: weight.medium },
  description: {
    marginTop: space.s0_5,
    color: color.fgMuted,
    lineHeight: leading.body,
    overflowWrap: "break-word",
  },
  subCaption: { marginTop: space.s0_5, color: color.fgFaint },
  dotSep: { lineHeight: 1, color: color.fgFaint },
  subCaptionMuted: { marginTop: space.s0_5, color: color.fgMuted },

  sectionHead: { marginBottom: space.s1_5, display: "flex", alignItems: "baseline", gap: space.s2 },
  sectionBody: { display: "grid", gap: space.s1 },
  sectionOuterPad: { paddingBlock: space.s3 },
  entry: {
    display: "flex",
    alignItems: "baseline",
    gap: space.s2,
    fontFamily: "var(--font-mono)",
  },
  entryPlain: { display: "flex", alignItems: "baseline", gap: space.s2 },
  statusPad: { paddingTop: space.s1, paddingBottom: space.s2 },
  padBottom: { paddingBottom: space.s2 },
  groupPad: { paddingBlock: space.s1_5 },
  afterTitle: { marginTop: space.s0_5 },
  matchCount: { marginInlineStart: space.s1_5, fontWeight: weight.regular, color: color.fgFaint },
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
  discloseRow: {
    display: "grid",
    gridTemplateColumns: "calc(var(--spacing) * 3.5) minmax(0, 1fr) auto",
    alignItems: "center",
    gap: space.s2,
    borderWidth: 0,
    backgroundColor: "transparent",
    textAlign: "left",
  },
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
  rowGlyph: { marginTop: space.s1 },
  toolBlurb: { display: "block", color: color.fgFaint },
  panelInset: { paddingTop: space.s1, paddingBottom: space.s3, paddingLeft: "58px" },
  fieldLabel: { color: color.fgMuted, fontWeight: weight.medium },
  afterLabel: { marginTop: space.s1 },
  footer: { paddingTop: space.s3_5, paddingBottom: "18px", lineHeight: leading.body },
});

/**
 * The code surfaces: a command log, a file, a diff.
 *
 * Their inset is NOT the dock gutter — it sits beside a line-number column and belongs to the
 * code.
 */
export const codeStyles = stylex.create({
  sheet: { paddingBlock: space.s2, fontFamily: "var(--font-mono)", lineHeight: leading.relaxed },
  /** Its block inset differs from `sheet`'s, which is why it is composed AFTER it, never before. */
  sheetInset: {
    display: "flex",
    flexDirection: "column",
    gap: space.s2_5,
    paddingInline: space.s3,
    paddingBlock: space.s3,
  },
  gutter: { textAlign: "right", color: color.fgFaint, userSelect: "none" },
  wrap: { minWidth: 0, whiteSpace: "pre-wrap", overflowWrap: "anywhere" },
  // `anywhere`, like `wrap` above and for the same reason: a hunk header carries the enclosing
  // declaration, and a generic signature or a long identifier has nowhere to break. Only the
  // break — the header's whitespace is already whatever the diff sent.
  hunk: {
    marginTop: space.s2_5,
    borderWidth: 0,
    backgroundColor: surface.sunken,
    paddingInline: space.s3,
    paddingBlock: space.s1,
    color: color.fgFaint,
    overflowWrap: "anywhere",
  },
  sideBySide: { display: "grid", gridTemplateColumns: "repeat(2, minmax(0, 1fr))" },
  blank: { backgroundColor: surface.sunken },

  // The gutter widths below are absolute on purpose — a digit column is sized by how many
  // digits must fit, which is not a rung of the spacing rhythm. (`matchRow` above says the
  // same 44px as `--spacing * 11`; that one is the odd spelling.)
  lineRow: { display: "grid", alignItems: "flex-start", paddingInline: space.s3 },
  gutterOne: { gridTemplateColumns: "44px minmax(0, 1fr)", gap: space.s2 },
  gutterPair: { gridTemplateColumns: "36px 36px minmax(0, 1fr)", gap: space.s1_5 },
  gutterSign: { gridTemplateColumns: "34px 16px minmax(0, 1fr)", gap: space.s1_5 },
  lineMeta: { textAlign: "right", userSelect: "none" },
  signMeta: { textAlign: "center", userSelect: "none" },
  targetLine: { backgroundColor: surface.accentWash },

  rowAdded: { backgroundColor: "var(--color-diff-added-tint)" },
  rowDeleted: { backgroundColor: "var(--color-diff-deleted-tint)" },
  metaAdded: { color: "var(--color-diff-added-meta)" },
  metaDeleted: { color: "var(--color-diff-deleted-meta)" },
  metaContext: { color: color.fgFaint },

  commandPlate: {
    borderRadius: radius.card,
    paddingInline: space.s3,
    paddingBlock: space.s2_5,
    transitionProperty: "background-color",
    transitionDuration: motion.color,
  },
  commandSelected: { backgroundColor: surface.selected },
  commandResting: { backgroundColor: surface.sunken },
  prompt: { flexShrink: 0, color: color.fgFaint },
  running: { flexShrink: 0, color: color.accent },
  failed: { flexShrink: 0, color: color.negative },
  output: {
    marginTop: space.s1_5,
    whiteSpace: "pre-wrap",
    overflowWrap: "break-word",
    color: color.fgMuted,
  },
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

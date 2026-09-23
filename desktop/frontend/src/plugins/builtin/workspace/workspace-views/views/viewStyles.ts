import * as stylex from "@stylexjs/stylex";
import { color, leading, motion, radius, space, surface, weight } from "@/styles/tokens.stylex";

/**
 * The shapes a dock view is made of.
 *
 * Type steps are NOT here: a view composes `typeStep.uiMd` and friends from the design
 * system, because a size is the design's vocabulary and this file is only the arrangement.
 */
export const viewStyles = stylex.create({
  gutter: { paddingInline: "var(--reading-gutter-wide)" },

  rowPad: { paddingBlock: space.s2 },
  rowPadTall: { paddingBlock: space.s2_5 },
  padBlockSm: { paddingBlock: space.s1 },
  sectionPad: { paddingBottom: space.s1 },
  sectionLabel: { paddingBlock: space.s2 },
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
    transitionTimingFunction: motion.easeState,
  },
  treeRowSelected: {
    backgroundColor: { default: surface.selected, ":hover": surface.selectedHover },
  },
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
    transitionTimingFunction: motion.easeState,
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
    transitionTimingFunction: motion.easeState,
    backgroundColor: { default: null, ":hover": surface.hover },
  },
  glyphTop: { marginTop: space.s0_5 },
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
  /**
   * The subject's own column, rather than a box the size of whatever is in it.
   *
   * The row is `[kind] [subject] [status] [duration] [clock]`, and the three on the right do not
   * shrink — so the subject is the one that has to, and it only did when the branch inside it
   * truncated for itself. `ToolText` does for prose and `FilePath` does for a path, which is why
   * a subject that reached neither ran on under the duration and the clock and painted them over.
   * The boundary belongs to the column — and it signs the cut, because a clip with no ellipsis
   * is a line the reader cannot tell was shortened. One line, always: a timeline row is a row.
   */
  subject: {
    minWidth: 0,
    flex: 1,
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
  },
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

  // Medium, as zcode sets a label or a row's name: weight marks it without shouting.
  title: { color: color.fg, fontWeight: weight.medium },
  description: {
    marginTop: space.s0_5,
    color: color.fgMuted,
    lineHeight: leading.body,
    overflowWrap: "break-word",
  },
  subCaption: { marginTop: space.s0_5, color: color.fgFaint },
  dotSep: { lineHeight: 1, color: color.fgFaint },
  subCaptionMuted: { marginTop: space.s0_5, color: color.fgMuted },

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
  // A measure wide enough for the durations a tool call produces, so the status mark to its
  // left lands on one edge down the list. Without it every row placed its own trailing cluster:
  // `42ms` and `120ms` differ by a character, and the glyph before them moved with it.
  duration: { minWidth: "calc(var(--spacing) * 12)", textAlign: "right" },
  runHeader: {
    display: "flex",
    minHeight: space.s10,
    alignItems: "flex-start",
    gap: space.s2,
    borderRadius: radius.card,
    backgroundColor: surface.sunken,
    marginInline: space.s2,
    paddingBlock: space.s2,
    paddingLeft: "calc(var(--reading-gutter-wide) - var(--spacing) * 2)",
    paddingRight:
      "calc(var(--reading-gutter-wide) - var(--spacing) * 2 - (var(--control-height-md) - var(--icon-md)) / 2)",
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
    borderLeftWidth: "var(--control-edge-width)",
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

/**
 * The code surfaces: a command log, a file, a diff.
 *
 * Their inset is NOT the dock gutter — it sits beside a line-number column and belongs to the
 * code.
 */
export const codeStyles = stylex.create({
  sheet: { paddingBlock: space.s2, fontFamily: "var(--font-mono)", lineHeight: leading.relaxed },
  gutter: { textAlign: "right", color: color.fgFaint, userSelect: "none" },
  wrap: { minWidth: 0, whiteSpace: "pre-wrap", overflowWrap: "anywhere" },
  // `anywhere`, like `wrap` above and for the same reason: a hunk header carries the enclosing
  // declaration, and a generic signature or a long identifier has nowhere to break. Only the
  // break — the header's whitespace is already whatever the diff sent.
  hunk: {
    // The gap separates one hunk from the one above it; the first has the file header there
    // instead, and took both — an 18px empty band under every card header.
    marginTop: { default: space.s2_5, ":first-child": 0 },
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

  // The tint has to stay under syntax colour and so stays low; the spine is opaque, carries no
  // text, and is what makes the row scannable at that alpha.
  rowAdded: {
    backgroundColor: "var(--color-diff-added-tint)",
    boxShadow: "var(--shadow-diff-added-spine)",
  },
  rowDeleted: {
    backgroundColor: "var(--color-diff-deleted-tint)",
    boxShadow: "var(--shadow-diff-deleted-spine)",
  },
  metaAdded: { color: "var(--color-diff-added-meta)" },
  metaDeleted: { color: "var(--color-diff-deleted-meta)" },
  metaContext: { color: color.fgFaint },

  fileCard: {
    marginBottom: space.s2,
    marginTop: { default: null, ":first-child": space.s2 },
    overflow: "hidden",
    borderRadius: radius.card,
    borderWidth: "var(--hairline-width)",
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

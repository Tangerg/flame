import * as stylex from "@stylexjs/stylex";
import { color, corner, leading, radius, space, surface, weight } from "@/styles/tokens.stylex";

/**
 * The fixtures' own scaffolding.
 *
 * These files are not the product — they are the frame the product is photographed in — but
 * they compile through the same pipeline, so Tailwind cannot be removed while they still speak
 * it. Sixty-eight class strings lived here, invisible to the migration's own scanner because it
 * only ever read `src`, and invisible to `check-dead-styles` for the same reason: a rule kept
 * alive by a fixture was reported as dead.
 *
 * One module for four files, deliberately. A fixture's job is to hold still, not to grow a
 * vocabulary.
 */
export const fx = stylex.create({
  // The shape a fixture is built from: a column that scrolls without stretching its parent.
  pane: { display: "flex", minHeight: 0, flex: 1, flexDirection: "column" },
  paneRow: { display: "flex", minHeight: 0, flex: 1 },
  scroller: {
    display: "flex",
    minHeight: 0,
    flex: 1,
    flexDirection: "column",
    gap: space.s0_5,
    overflowY: "auto",
    paddingInline: space.s2,
    paddingTop: space.s2,
  },
  listPad: { paddingInline: space.s2, paddingTop: space.s2 },
  column: { display: "flex", flexDirection: "column" },
  columnTight: { display: "flex", flexDirection: "column", gap: space.s0_5 },
  line: { display: "flex", alignItems: "center" },
  contents: { display: "contents" },
  relative: { position: "relative" },
  fill: { flex: 1 },

  /** The caption over a fixture's state list. */
  listHead: {
    paddingInline: space.s2,
    paddingBottom: space.s1,
    fontWeight: weight.semibold,
    color: color.fg,
  },
  /** The note at the bottom that says what the fixture is. */
  listFoot: {
    paddingInline: space.s4,
    paddingBottom: space.s3,
    lineHeight: leading.body,
    color: color.fgFaint,
  },
  /** The gap above that note, which is a gap and not a spring. */
  footGap: { minHeight: space.s4 },

  // Ink and weight, as the fixtures use them.
  ink: { color: color.fg },
  soft: { color: color.fgSoft },
  muted: { color: color.fgMuted },
  faint: { color: color.fgFaint },
  medium: { fontWeight: weight.medium },
  semibold: { fontWeight: weight.semibold },
  // Both of these carry `letter-spacing`, so they must come AFTER the type step in
  // `stylex.props` — a type step declares tracking too, and composed last it replaces theirs
  // silently. Monospace at a proportional face's tracking drifts; an uppercase label without
  // wide tracking sets solid.
  mono: { fontFamily: "var(--font-mono)", letterSpacing: 0 },
  figures: { fontVariantNumeric: "tabular-nums" },
  truncate: { overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" },
  relaxed: { lineHeight: leading.relaxed },

  /** A section label in the foundation fixture's specimen list. */
  specimenLabel: {
    paddingInline: space.s2,
    paddingBottom: space.s1,
    fontWeight: weight.medium,
    letterSpacing: "var(--tracking-wide)",
    textTransform: "uppercase",
    color: color.fgFaint,
  },
  specimenLabelFlush: {
    fontWeight: weight.medium,
    letterSpacing: "var(--tracking-wide)",
    textTransform: "uppercase",
    color: color.fgFaint,
  },
  /** A specimen card: the one place a fixture draws a box, to show a surface against an edge. */
  card: {
    borderRadius: radius.lg,
    borderWidth: "1px",
    borderStyle: "solid",
    borderColor: surface.field,
    backgroundColor: surface.surface,
    padding: space.s4,
  },
  cardGrid: {
    marginTop: space.s8,
    display: "grid",
    gridTemplateColumns: "repeat(2, 1fr)",
    gap: space.s4,
  },

  /** The foundation fixture's reading column, at the product's own measure and gutter. */
  measure: {
    marginInline: "auto",
    display: "flex",
    width: "100%",
    maxWidth: "var(--content-max)",
    flex: 1,
    flexDirection: "column",
    paddingTop: space.s10,
    paddingInline: {
      default: "var(--density-column-gutter)",
      "@media (min-width: 640px)": "var(--density-column-gutter-wide)",
    },
  },
  heading: {
    marginTop: space.s2,
    fontWeight: weight.semibold,
    lineHeight: leading.tight,
    color: color.fg,
  },
  headingLoose: {
    marginTop: space.s4,
    fontWeight: weight.semibold,
    lineHeight: leading.tight,
    color: color.fg,
  },
  lede: { marginTop: space.s3, maxWidth: "62ch", lineHeight: leading.relaxed, color: color.fgSoft },
  afterHeading: { marginTop: space.s2, lineHeight: leading.relaxed, color: color.fgMuted },
  row: { marginTop: space.s3, display: "flex", alignItems: "center", gap: space.s2 },
  rowBaseline: {
    marginTop: space.s3,
    display: "flex",
    alignItems: "flex-end",
    gap: space.s3,
    color: color.fg,
  },
  captionRow: {
    display: "flex",
    alignItems: "center",
    gap: space.s1,
    paddingInline: space.s2,
    paddingBottom: space.s2,
  },

  /** The foundation fixture's stand-in composer, at the product's density insets. */
  editorBox: {
    minHeight: "calc(var(--spacing) * 20)",
    paddingInline: "var(--density-composer-editor-start)",
    paddingTop: "var(--density-composer-editor-top)",
    paddingBottom: "var(--density-composer-editor-bottom)",
    lineHeight: leading.relaxed,
    color: color.fg,
  },
  footerBox: {
    display: "flex",
    alignItems: "center",
    gap: space.s1,
    paddingInline: "var(--density-composer-footer)",
    paddingBottom: "var(--density-composer-footer)",
  },
  minRail: { minWidth: space.s2, flex: 1 },
  minField: { minHeight: space.s8, flex: 1 },
  hairline: { height: space.s4, flexShrink: 0 },

  /** The shell fixture's empty state, centred in the pane. */
  emptyBox: {
    margin: "auto",
    display: "flex",
    maxWidth: "520px",
    flexDirection: "column",
    alignItems: "center",
    paddingInline: space.s8,
    textAlign: "center",
  },
  // 40px, and the ONE element whose corner shape a golden can see: the product sets
  // `corner-shape: superellipse(1.5)` on everything, so a circle needs `corner.pill` to opt
  // back out. Removing the Tailwind class this used to carry turned it into a squircle.
  emptyGlyph: {
    display: "grid",
    height: space.s10,
    width: space.s10,
    placeItems: "center",
    backgroundColor: surface.surface2,
    color: color.fgMuted,
  },
});

export { corner };

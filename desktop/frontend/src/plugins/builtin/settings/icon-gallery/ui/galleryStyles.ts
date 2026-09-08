import * as stylex from "@stylexjs/stylex";
import { color, motion, radius, space, surface, weight } from "@/styles/tokens.stylex";

/**
 * The gallery card, at the two measures its two panes use.
 *
 * Both panes render the same thing — a plate holding a glyph, its name under it — and had
 * written it out twice at 120/44px and 96/34px. The card is the same card; only the measure
 * differs, so that is the only thing the two steps carry.
 */
export const galleryStyles = stylex.create({
  card: {
    display: "flex",
    // A gallery entry is a specimen, not a control: the pointer says nothing about pressing it.
    cursor: "default",
    flexDirection: "column",
    alignItems: "center",
    gap: space.s1_5,
    borderRadius: radius.card,
    backgroundColor: { default: surface.card, ":hover": surface.hover },
    transitionProperty: "background-color",
    transitionDuration: motion.fast,
  },
  cardLarge: { paddingInline: space.s2_5, paddingTop: space.s3_5, paddingBottom: space.s2_5 },
  cardSmall: { paddingInline: space.s2, paddingTop: space.s2_5, paddingBottom: space.s2 },

  plate: {
    display: "grid",
    placeItems: "center",
    backgroundColor: surface.surface2,
    color: color.fg,
  },
  plateLarge: { height: space.s11, width: space.s11, borderRadius: radius.card },
  plateSmall: {
    height: "calc(var(--spacing) * 8.5)",
    width: "calc(var(--spacing) * 8.5)",
    borderRadius: radius.sm,
  },

  name: {
    maxWidth: "100%",
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
    textAlign: "center",
    color: color.fg,
    fontWeight: weight.medium,
  },
  missing: { fontFamily: "var(--font-mono)", color: color.fgFaint },

  /** A section's heading: what the group is, and how many are in it. */
  sectionHead: {
    display: "flex",
    alignItems: "baseline",
    justifyContent: "space-between",
    fontFamily: "var(--font-mono)",
    // Mono at the UI tracking reads loose; a group heading is the one place it shows.
    letterSpacing: "normal",
    fontWeight: weight.medium,
    color: color.fgMuted,
  },
  count: { fontFamily: "var(--font-mono)", color: color.fgFaint },
});

/** Auto-fill so the grid decides its own column count from the pane's width. */
export const gallerySpread = stylex.create({
  large: {
    display: "grid",
    gap: space.s2,
    gridTemplateColumns: "repeat(auto-fill, minmax(120px, 1fr))",
  },
  small: {
    display: "grid",
    gap: space.s1_5,
    gridTemplateColumns: "repeat(auto-fill, minmax(96px, 1fr))",
  },
});

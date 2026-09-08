import * as stylex from "@stylexjs/stylex";
import { color, leading, radius, space, surface, weight } from "@/styles/tokens.stylex";

/**
 * The transcript's own arrangement.
 *
 * Unlike the dock and the settings panes, this plugin has no dominant repeated shape — its
 * classes are mostly one-off layout for one card each. What is shared is the vocabulary a
 * card is built from: a line that truncates, a part that yields its width, an ink step.
 */
export const chatStyles = stylex.create({
  line: { display: "flex", alignItems: "center", gap: space.s2 },
  lineTight: { display: "flex", alignItems: "center", gap: space.s1_5 },
  column: { display: "flex", flexDirection: "column" },
  stackTight: { display: "flex", flexDirection: "column", gap: space.s1_5 },
  stackHairline: { display: "flex", flexDirection: "column", gap: space.s0_5 },
  split: { display: "flex", alignItems: "center", justifyContent: "space-between", gap: space.s2 },
  fill: { minWidth: 0, flex: 1 },
  min: { minWidth: 0 },
  hold: { flexShrink: 0 },
  truncate: { overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" },
  centre: { textAlign: "center" },
  pretty: { textWrap: "pretty" },
  ink: { color: color.fg },
  soft: { color: color.fgSoft },
  muted: { color: color.fgMuted },
  faint: { color: color.fgFaint },
  accent: { color: color.accent },
  negative: { color: color.negative },
  mono: { fontFamily: "var(--font-mono)" },
  figures: { fontVariantNumeric: "tabular-nums" },
  medium: { fontWeight: weight.medium },
  strong: { fontWeight: weight.semibold },
  /** A reply preview: three lines at most, because the rail is a glance and not the message. */
  clampOne: {
    display: "-webkit-box",
    WebkitBoxOrient: "vertical",
    WebkitLineClamp: 1,
    overflow: "hidden",
  },
  clampThree: {
    display: "-webkit-box",
    WebkitBoxOrient: "vertical",
    WebkitLineClamp: 3,
    overflow: "hidden",
  },
  bodyLeading: { lineHeight: leading.body },
  snugLeading: { lineHeight: leading.snug },
  /** A row inside a floating panel, which takes the panel's own highlight rather than hover. */
  panelRow: {
    display: "flex",
    flexDirection: "column",
    gap: space.s0_5,
    borderRadius: radius.sm,
    backgroundColor: { default: null, ":is([data-highlighted])": surface.surface2 },
    paddingInline: space.s2_5,
    paddingBlock: space.s1_5,
    outline: "none",
  },
});

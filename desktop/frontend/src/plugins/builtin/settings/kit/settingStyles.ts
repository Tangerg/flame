import * as stylex from "@stylexjs/stylex";
import { color, leading, motion, radius, space, surface, weight } from "@/styles/tokens.stylex";

export const settingStyles = stylex.create({
  stack: { display: "flex", flexDirection: "column", gap: space.s3 },

  split: { display: "flex", alignItems: "center", justifyContent: "space-between", gap: space.s3 },
  lineWide: { display: "flex", alignItems: "center", gap: space.s3 },
  lineWrap: { display: "flex", flexWrap: "wrap", alignItems: "center", gap: space.s1_5 },
  end: { display: "flex", justifyContent: "flex-end" },

  label: { color: color.fg, fontWeight: weight.medium },
  hint: { color: color.fgMuted, lineHeight: leading.snug },
  hintSpaced: { marginTop: space.s1, color: color.fgMuted, lineHeight: leading.body },

  hoverRow: {
    borderRadius: radius.card,
    backgroundColor: { default: null, ":hover": surface.hover },
    paddingInline: space.s3,
    paddingBlock: space.s2_5,
    transitionProperty: "background-color",
    transitionTimingFunction: motion.easeState,
  },
  hoverRowTall: { paddingBlock: space.s3 },
  hoverRowTight: {
    marginInline: "calc(var(--spacing) * -2)",
    paddingInline: space.s2,
    paddingBlock: space.s2,
  },
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
  caption: { marginBottom: space.s1_5, color: color.fgMuted, fontWeight: weight.medium },
  inline: { display: "inline-flex", alignItems: "center", gap: space.s1 },
  grid2: { display: "grid", gap: space.s2 },
  selfEnd: { alignSelf: "flex-end" },
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
    transitionTimingFunction: motion.easeState,
  },
});

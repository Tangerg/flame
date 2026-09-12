import * as stylex from "@stylexjs/stylex";
import { color, leading, radius, space, surface } from "@/styles/tokens.stylex";

export const previewStyles = stylex.create({
  row: {
    borderRadius: radius.step2xs,
    backgroundColor: { default: null, ":hover": surface.hover },
    paddingInline: space.s1,
    transitionProperty: "background-color",
  },
  rowPad: { paddingBlock: space.s0_5 },
  numbered: {
    display: "grid",
    gridTemplateColumns: "calc(var(--spacing) * 7) minmax(0, 1fr)",
    alignItems: "flex-start",
    gap: space.s2_5,
  },
  numberedWide: { gridTemplateColumns: "3rem minmax(0, 1fr)", gap: space.s2 },
  gutter: {
    textAlign: "right",
    color: color.fgFaint,
    fontVariantNumeric: "tabular-nums",
    userSelect: "none",
  },
  wrap: { minWidth: 0, whiteSpace: "pre-wrap", overflowWrap: "anywhere" },
  wrapWords: { minWidth: 0, whiteSpace: "pre-wrap", overflowWrap: "break-word" },
  sheet: { fontFamily: "var(--font-mono)", lineHeight: leading.body },
  inset: { paddingTop: space.s1 },
  insetTight: { paddingTop: space.s0_5 },
});

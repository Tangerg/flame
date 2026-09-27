import * as stylex from "@stylexjs/stylex";
import { leading, radius, space, surface, weight } from "@/styles/tokens.stylex";

export const chatStyles = stylex.create({
  centre: { textAlign: "center" },
  medium: { fontWeight: weight.medium },
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
  panelRow: {
    display: "flex",
    flexDirection: "column",
    gap: space.s0_5,
    borderRadius: radius.sm,
    backgroundColor: { default: null, ":is([data-highlighted])": surface.surface2 },
    paddingInline: space.s2_5,
    paddingBlock: space.s1_5,
  },
});

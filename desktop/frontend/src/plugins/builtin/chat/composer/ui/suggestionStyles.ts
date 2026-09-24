import * as stylex from "@stylexjs/stylex";
import { color, space, weight } from "@/styles/tokens.stylex";

export const suggestionStyles = stylex.create({
  panel: {
    display: "flex",
    maxHeight: "min(320px, var(--available-height))",
    flexDirection: "column",
    padding: space.s1,
  },
  list: { minHeight: 0, overflowY: "auto", overscrollBehavior: "contain" },
  state: {
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
    gap: space.s2,
    paddingInline: space.s2_5,
    paddingBlock: space.s2,
    color: color.fgMuted,
  },
  note: {
    margin: 0,
    paddingInline: space.s2_5,
    paddingTop: space.s1_5,
    paddingBottom: space.s1,
    color: color.fgFaint,
  },
  heading: { paddingInline: space.s2_5, paddingTop: space.s1_5, paddingBottom: space.s1 },
  command: {
    borderWidth: 0,
    backgroundColor: "transparent",
    padding: 0,
    fontFamily: "var(--font-mono)",
    fontWeight: weight.medium,
    color: color.accent,
  },
  directory: { color: color.fgFaint },
  name: { fontWeight: weight.medium, color: color.fg },
});

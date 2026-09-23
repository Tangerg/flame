import * as stylex from "@stylexjs/stylex";
import { color, space, weight } from "@/styles/tokens.stylex";

export const suggestionStyles = stylex.create({
  panel: { padding: space.s1 },
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

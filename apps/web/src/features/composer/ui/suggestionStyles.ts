import * as stylex from "@stylexjs/stylex";
import { color, leading, space, weight } from "@flame/ui/tokens.stylex";
export const suggestionStyles = stylex.create({
  panel: {
    display: "flex",
    maxHeight: "min(320px, var(--available-height))",
    flexDirection: "column",
    padding: space.s1,
  },
  list: { minHeight: 0, overflowY: "auto", overscrollBehavior: "contain" },
  // The status row is the panel child constrained by its available height, so
  // it scrolls like the list instead of clipping the error or Retry.
  state: {
    display: "flex",
    minHeight: 0,
    alignItems: "center",
    justifyContent: "space-between",
    gap: space.s2,
    paddingInline: space.s2_5,
    paddingBlock: space.s2,
    overflowY: "auto",
    overscrollBehavior: "contain",
    color: color.fgMuted,
  },
  stateText: {
    display: "flex",
    minWidth: 0,
    flexDirection: "column",
    gap: space.s0_5,
    overflowWrap: "anywhere",
  },
  heading: { paddingInline: space.s2_5, paddingTop: space.s1_5, paddingBottom: space.s1 },
  command: {
    borderWidth: 0,
    backgroundColor: "transparent",
    padding: 0,
    fontFamily: "var(--font-mono)",
    fontWeight: weight.medium,
    color: color.accentText,
  },
  path: {
    display: "flex",
    flexDirection: "column-reverse",
    gap: space.s0_5,
    lineHeight: leading.snug,
  },
  directory: { color: color.fgFaint, whiteSpace: "normal", overflowWrap: "anywhere" },
  name: { maxWidth: "100%", fontWeight: weight.medium, color: color.fg },
});

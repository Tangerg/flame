import * as stylex from "@stylexjs/stylex";
import { color, leading, space, weight } from "@/styles/tokens.stylex";

export const toolbarStyles = stylex.create({
  describedRow: { alignItems: "flex-start", paddingBlock: space.s1_5 },
  optionTitle: { display: "block", fontWeight: weight.medium, color: color.fg },
  optionDetail: { display: "block", lineHeight: leading.snug, color: color.fgMuted },
  checkTop: { marginTop: space.s0_5 },
  capitalize: { textTransform: "capitalize" },
});

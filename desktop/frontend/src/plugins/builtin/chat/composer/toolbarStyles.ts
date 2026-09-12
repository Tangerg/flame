import * as stylex from "@stylexjs/stylex";
import { color, leading, space, weight } from "@/styles/tokens.stylex";

/** The row's shape, hover, corner and outline all belong
 *  to `floatingRow` — what is here is only what these two rows add to it. */
export const toolbarStyles = stylex.create({
  describedRow: { alignItems: "flex-start", paddingBlock: space.s1_5 },
  optionTitle: { display: "block", fontWeight: weight.semibold, color: color.fg },
  optionDetail: { display: "block", lineHeight: leading.snug, color: color.fgMuted },
  checkTop: { marginTop: space.s0_5 },
  capitalize: { textTransform: "capitalize" },
});

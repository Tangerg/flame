import * as stylex from "@stylexjs/stylex";
import { color, leading, space, weight } from "@/styles/tokens.stylex";

/** The composer toolbar's own two menus. The row's shape, hover, corner and outline all belong
 *  to `floatingRow` — what is here is only what these two rows add to it. */
export const toolbarStyles = stylex.create({
  /** An option with a description under it: the check aligns to the first line, not the box. */
  describedRow: { alignItems: "flex-start", paddingBlock: space.s1_5 },
  optionTitle: { display: "block", fontWeight: weight.semibold, color: color.fg },
  optionDetail: { display: "block", lineHeight: leading.snug, color: color.fgMuted },
  /** The check beside a two-line option sits on the title's line. */
  checkTop: { marginTop: space.s0_5 },
  capitalize: { textTransform: "capitalize" },
});

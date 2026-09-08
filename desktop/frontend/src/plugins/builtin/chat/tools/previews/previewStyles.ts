import * as stylex from "@stylexjs/stylex";
import { color, leading, radius, space, surface } from "@/styles/tokens.stylex";

/**
 * A tool preview's rows.
 *
 * Seven previews — a file, a glob, a grep, an LSP result, a recall hit, a skill — render the
 * same row: the tightest corner the ladder has, a one-step inset, and the row wash under the
 * pointer. Each had written it out, and two of them at a different corner.
 */
export const previewStyles = stylex.create({
  row: {
    borderRadius: radius.step2xs,
    backgroundColor: { default: null, ":hover": surface.hover },
    paddingInline: space.s1,
    transitionProperty: "background-color",
  },
  rowPad: { paddingBlock: space.s0_5 },
  /** A numbered row: the number holds a column so every line of code starts level. */
  numbered: {
    display: "grid",
    gridTemplateColumns: "calc(var(--spacing) * 7) minmax(0, 1fr)",
    alignItems: "flex-start",
    gap: space.s2_5,
  },
  numberedWide: { gridTemplateColumns: "3rem minmax(0, 1fr)", gap: space.s2 },
  /** A line number is counted against, never read — so it takes no selection. */
  gutter: {
    textAlign: "right",
    color: color.fgFaint,
    fontVariantNumeric: "tabular-nums",
    userSelect: "none",
  },
  /** Machine text wraps rather than scrolling: a long line is still one line of meaning. */
  wrap: { minWidth: 0, whiteSpace: "pre-wrap", overflowWrap: "anywhere" },
  wrapWords: { minWidth: 0, whiteSpace: "pre-wrap", overflowWrap: "break-word" },
  sheet: { fontFamily: "var(--font-mono)", lineHeight: leading.body },
  /** A preview sits under the row that names it, so it owns the gap between them. */
  inset: { paddingTop: space.s1 },
  insetTight: { paddingTop: space.s0_5 },
});

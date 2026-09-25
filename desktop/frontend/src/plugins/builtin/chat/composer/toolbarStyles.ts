import * as stylex from "@stylexjs/stylex";
import { color, leading, space, weight } from "@/styles/tokens.stylex";

export const toolbarStyles = stylex.create({
  describedRow: { alignItems: "flex-start", paddingBlock: space.s1_5 },
  optionTitle: { display: "block", fontWeight: weight.medium, color: color.fg },
  optionDetail: { display: "block", lineHeight: leading.snug, color: color.fgMuted },
  // A described row is top-aligned because its second line must not drag the
  // glyphs down. Each glyph therefore carries the title's own line box and
  // centres inside it, so alignment follows the type scale instead of a nudge
  // that only holds at one font size. `Icon` sizes itself inline, so the box
  // has to be a separate element.
  optionGlyphBox: { display: "grid", height: "1lh", placeItems: "center" },
  optionGlyph: { color: color.fgMuted },
  menuHeading: {
    paddingInline: space.s2,
    paddingTop: space.s1,
    paddingBottom: space.s0_5,
    fontWeight: weight.medium,
    color: color.fgFaint,
  },
});

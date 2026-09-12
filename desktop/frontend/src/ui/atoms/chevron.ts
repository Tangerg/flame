import * as stylex from "@stylexjs/stylex";

/**
 * The glyph that says whether something is open, and turns when it changes.
 *
 * The INK is not here. A tree's chevron is faint, a menu's inherits the row it sits in, and the
 * diff header's steps back by `--glyph-step`; each call site says which.
 *
 * `rotate` and not `transform`: they are separate properties, and a transform on the same glyph
 * (a press scale, say) would otherwise be replaced rather than composed.
 */
export const chevron = stylex.create({
  base: { flexShrink: 0, transitionProperty: "rotate" },
  shut: { rotate: "-90deg" },
});

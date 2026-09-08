import * as stylex from "@stylexjs/stylex";

/**
 * The glyph that says whether something is open, and turns when it changes.
 *
 * This was eight call sites in four spellings: `rotate: "-90deg"` with
 * `transitionProperty: "rotate"`, the same pair without the transition, a permanently turned
 * one on the select trigger, and three `cn("shrink-0 transition-transform", !open &&
 * "-rotate-90")`. All eight mean one thing — a chevron points down when open and right when
 * shut — so the disagreements between them were never decisions.
 *
 * The INK is not here. A tree's chevron is faint, a menu's inherits the row it sits in, and the
 * diff header's steps back by `--glyph-step`; each call site says which, because that is the
 * part they genuinely differ on.
 *
 * `rotate` and not `transform`: they are separate properties, and a transform on the same glyph
 * (a press scale, say) would otherwise be replaced rather than composed.
 */
export const chevron = stylex.create({
  /** Holds its width beside a label that may truncate, and animates only its turn. */
  base: { flexShrink: 0, transitionProperty: "rotate" },
  /** Pointing at what it would open. */
  shut: { rotate: "-90deg" },
});

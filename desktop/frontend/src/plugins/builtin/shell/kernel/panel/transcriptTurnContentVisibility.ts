import * as stylex from "@stylexjs/stylex";
import type { StyleXStyles } from "@stylexjs/stylex";

const offscreen = stylex.create({
  skip: { contentVisibility: "auto", containIntrinsicSize: "auto 220px" },
});

/**
 * The TAIL turn may never opt into the off-screen rendering skip: it owns the current Run
 * outcome or HITL action, and a cold restore can place that action in the viewport before
 * Chrome has measured it. content-visibility there leaves only the intrinsic placeholder in
 * layout and drops the real controls from the accessibility tree, so even an exact
 * scroll-to-bottom cannot reveal them.
 *
 * A STYLE rather than a class name, and the difference is not cosmetic: as two Tailwind
 * arbitrary-property classes this was the only styling in the transcript that no compiler
 * checked. It outlived the utility it was written against, and what a dead class removes is
 * containment — which changes nothing anyone can see and everything about how the turn below
 * it rasterises. The caller composes it into its own `stylex.props`, so the containment and
 * the gutter that positions it resolve against each other instead of racing in `cn()`.
 */
export function transcriptTurnContentVisibility(isLast: boolean): StyleXStyles | undefined {
  return isLast ? undefined : offscreen.skip;
}

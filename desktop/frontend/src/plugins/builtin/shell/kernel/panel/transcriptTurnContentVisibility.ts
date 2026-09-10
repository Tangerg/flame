import * as stylex from "@stylexjs/stylex";
import type { StyleXStyles } from "@stylexjs/stylex";

// The 220px is a first-pass ESTIMATE for a turn nobody has scrolled to yet, and it had no
// recorded basis. Measured at scale — 304 turns, real heights ranging 28px to 528px — the
// property that matters holds: content stayed put through a full scroll to the top, moving more
// than 24px on 2 of 40 round trips, because the browser anchors scrolling to what is on screen.
// What the estimate being high costs is the SCROLLBAR: total height came out 70143px and
// settled at 53866px as history rendered, a 23% shrink, so the thumb grows while a long session
// is scrolled back through.
//
// Deliberately not re-tuned to the ~177px average that implies. That average is one fixture's
// content mix, and turn heights here already span twentyfold; fitting the number to this sample
// would be overfitting, and `auto` means each turn's real size replaces the estimate once it has
// been seen. Recorded rather than changed, so the next person has the measurement instead of an
// unexplained number.
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

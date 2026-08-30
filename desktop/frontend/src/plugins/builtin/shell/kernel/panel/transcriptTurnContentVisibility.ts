/**
 * The TAIL turn may never opt into the off-screen rendering skip: it owns the current Run
 * outcome or HITL action, and a cold restore can place that action in the viewport before
 * Chrome has measured it. content-visibility there leaves only the intrinsic placeholder in
 * layout and drops the real controls from the accessibility tree, so even an exact
 * scroll-to-bottom cannot reveal them.
 */
export function transcriptTurnContentVisibility(isLast: boolean): string | undefined {
  return isLast ? undefined : "[content-visibility:auto] [contain-intrinsic-size:auto_220px]";
}

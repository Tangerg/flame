import type { CSSProperties } from "react";
import { DOCK_MIN_WIDTH_PX, DOCK_SAFE_AREA_PX } from "@/lib/shellGeometry";

// A custom property on the ROW rather than a React style on the dock: the resizer writes it
// on every pointer-move and the flank follows in CSS with no render, so a drag costs one
// re-render on release instead of one per pointer event.
export const DOCK_RATIO_PROPERTY = "--dock-ratio";

/**
 * The whole range is spelled in CSS, so a window resize re-derives the measure with no
 * React render at all — the property the row carries is the person's POSITION in that
 * range, which a resize does not change.
 *
 * The percentages are deliberately left UNRESOLVED: they substitute where this is declared
 * (the row) but resolve on the flank, against the row either way, since it is both the flex
 * container and the containing block. That is what lets one value serve two properties
 * whose percentages must agree: the column's basis, and the negative end margin that slides
 * it out.
 */
const DOCK_MEASURE_PROPERTY = "--dock-measure";
const DOCK_USABLE_MAX = `max(${DOCK_MIN_WIDTH_PX}px, calc(100% - ${DOCK_SAFE_AREA_PX}px))`;
const DOCK_USABLE_MIN = `min(${DOCK_MIN_WIDTH_PX}px, ${DOCK_USABLE_MAX})`;
const DOCK_MEASURE = `calc(${DOCK_USABLE_MIN} + var(${DOCK_RATIO_PROPERTY}) * (${DOCK_USABLE_MAX} - ${DOCK_USABLE_MIN}))`;

/** Row style carrying the dock's geometry: where in its range the person put the flank,
 *  and the measure it keeps whether it is showing or gone. */
export function dockWidthRow(ratio: number): CSSProperties {
  return {
    [DOCK_RATIO_PROPERTY]: `${ratio}`,
    [DOCK_MEASURE_PROPERTY]: DOCK_MEASURE,
  } as CSSProperties;
}

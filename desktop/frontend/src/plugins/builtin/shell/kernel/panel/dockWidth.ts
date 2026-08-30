import type { CSSProperties } from "react";
import { CHAT_MIN_WIDTH_PX, DOCK_MIN_WIDTH_PX } from "@/lib/shellGeometry";

// A custom property on the ROW rather than a React style on the dock: the resizer writes it
// on every pointer-move and the flank follows in CSS with no render, so a drag costs one
// re-render on release instead of one per pointer event.
export const DOCK_WIDTH_PROPERTY = "--dock-width";

/**
 * Spelled in CSS so a stored width follows a window resize without a React render. The
 * percentages are deliberately left UNRESOLVED — `var(--dock-width)` substitutes at the row
 * where this is declared, while `50%`/`100%` survive as tokens and resolve on the flank.
 * That is what lets one value serve two properties whose percentages must agree: the
 * column's basis, and the negative end margin that slides it out.
 */
const DOCK_MEASURE_PROPERTY = "--dock-measure";
const DOCK_MEASURE = `max(${DOCK_MIN_WIDTH_PX}px, min(var(${DOCK_WIDTH_PROPERTY}), 50%, calc(100% - ${CHAT_MIN_WIDTH_PX}px)))`;

/** Row style carrying the dock's geometry: the width a drag starts from, and the
 *  measure the flank keeps whether it is showing or gone. */
export function dockWidthRow(width: number): CSSProperties {
  return {
    [DOCK_WIDTH_PROPERTY]: `${width}px`,
    [DOCK_MEASURE_PROPERTY]: DOCK_MEASURE,
  } as CSSProperties;
}

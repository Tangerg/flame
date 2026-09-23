import type { CSSProperties } from "react";
import { DOCK_MIN_WIDTH_PX, DOCK_SAFE_AREA_PX, DOCK_PREFERRED_WIDTH_PX } from "@/lib/shellGeometry";

export const DOCK_MEASURE_PROPERTY = "--dock-measure";
const DOCK_USABLE_MAX = `max(${DOCK_MIN_WIDTH_PX}px, calc(100% - ${DOCK_SAFE_AREA_PX}px))`;

export function dockWidthMeasure(ratio: number | null): string {
  return ratio === null
    ? `min(${DOCK_PREFERRED_WIDTH_PX}px, ${DOCK_USABLE_MAX})`
    : `calc(${DOCK_MIN_WIDTH_PX}px + ${ratio} * (${DOCK_USABLE_MAX} - ${DOCK_MIN_WIDTH_PX}px))`;
}

export function dockWidthRow(ratio: number | null): CSSProperties {
  return { [DOCK_MEASURE_PROPERTY]: dockWidthMeasure(ratio) } as CSSProperties;
}

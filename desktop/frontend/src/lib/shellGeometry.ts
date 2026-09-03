export const SIDEBAR_MIN_WIDTH_PX = 240;
export const SIDEBAR_DEFAULT_WIDTH_PX = 275;
const SIDEBAR_MAX_WIDTH_PX = 520;
const SIDEBAR_READING_MIN_WIDTH_PX = 240;

export const DOCK_MIN_WIDTH_PX = 320;
/** Reserved for the CONVERSATION, not the flank, before the flank may claim anything. */
export const DOCK_SAFE_AREA_PX = 352;
const DOCK_PREFERRED_WIDTH_PX = 640;
const DOCK_PREFERRED_SAFE_AREA_PX = 500;
const DOCK_ASPECT_RATIO = 16 / 10;

export function canPresentDock(rowWidth: number): boolean {
  return rowWidth >= DOCK_MIN_WIDTH_PX + DOCK_SAFE_AREA_PX;
}

export function clampSidebarWidth(width: number, shellWidth: number): number {
  return Math.round(Math.min(maxSidebarWidth(shellWidth), Math.max(SIDEBAR_MIN_WIDTH_PX, width)));
}

export function maxSidebarWidth(shellWidth: number): number {
  return Math.max(
    SIDEBAR_MIN_WIDTH_PX,
    Math.min(SIDEBAR_MAX_WIDTH_PX, shellWidth - SIDEBAR_READING_MIN_WIDTH_PX),
  );
}

/** SOLE owner of the floor — which is why the narrow end of the range is the constant, not a
 *  function. A second clamp on that end can never fire; one existed and did nothing. */
export function maxDockWidth(rowWidth: number): number {
  return Math.max(DOCK_MIN_WIDTH_PX, rowWidth - DOCK_SAFE_AREA_PX);
}

export function clampDockWidth(width: number, rowWidth: number): number {
  const max = maxDockWidth(rowWidth);
  return Math.round(Math.max(DOCK_MIN_WIDTH_PX, Math.min(width, max)));
}

function clamp01(ratio: number): number {
  return Number.isFinite(ratio) ? Math.max(0, Math.min(1, ratio)) : 1;
}

/** The persisted preference is this RATIO, never a width: the px measure changes with every
 *  window resize, and CSS re-derives it from the ratio with no React render. */
export function dockWidthFromRatio(ratio: number, rowWidth: number): number {
  const max = maxDockWidth(rowWidth);
  return Math.round(DOCK_MIN_WIDTH_PX + clamp01(ratio) * (max - DOCK_MIN_WIDTH_PX));
}

export function dockRatioFromWidth(width: number, rowWidth: number): number {
  const max = maxDockWidth(rowWidth);
  // Zero-width range: a row under floor + safe area has one legal width.
  if (max <= DOCK_MIN_WIDTH_PX) return 1;
  return clamp01((clampDockWidth(width, rowWidth) - DOCK_MIN_WIDTH_PX) / (max - DOCK_MIN_WIDTH_PX));
}

/** Widest measure that all three claims allow: the floor, a share of the window's HEIGHT
 *  (so a tall narrow window does not open a full-height flank), and the preferred width. */
export function defaultDockWidth(rowWidth: number, shellHeight: number): number {
  return Math.max(
    DOCK_MIN_WIDTH_PX,
    Math.min(shellHeight * DOCK_ASPECT_RATIO, rowWidth - DOCK_PREFERRED_SAFE_AREA_PX),
    Math.min(DOCK_PREFERRED_WIDTH_PX, rowWidth - DOCK_SAFE_AREA_PX),
  );
}

export function defaultDockRatio(rowWidth: number, shellHeight: number): number {
  return dockRatioFromWidth(defaultDockWidth(rowWidth, shellHeight), rowWidth);
}

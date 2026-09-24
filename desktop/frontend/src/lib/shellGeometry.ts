export const SIDEBAR_MIN_WIDTH_PX = 240;
export const SIDEBAR_DEFAULT_WIDTH_PX = 275;
const SIDEBAR_MAX_WIDTH_PX = 520;

export const DOCK_MIN_WIDTH_PX = 320;
export const CONVERSATION_READING_MIN_PX = 440;
export const DOCK_PREFERRED_WIDTH_PX = 480;

export function canPresentDock(rowWidth: number): boolean {
  return rowWidth >= DOCK_MIN_WIDTH_PX + CONVERSATION_READING_MIN_PX;
}

export function clampSidebarWidth(width: number, shellWidth: number): number {
  return Math.round(Math.min(maxSidebarWidth(shellWidth), Math.max(SIDEBAR_MIN_WIDTH_PX, width)));
}

export function maxSidebarWidth(shellWidth: number): number {
  return Math.max(
    SIDEBAR_MIN_WIDTH_PX,
    Math.min(SIDEBAR_MAX_WIDTH_PX, shellWidth - CONVERSATION_READING_MIN_PX),
  );
}

export function maxDockWidth(rowWidth: number): number {
  return Math.max(DOCK_MIN_WIDTH_PX, rowWidth - CONVERSATION_READING_MIN_PX);
}

export function clampDockWidth(width: number, rowWidth: number): number {
  const max = maxDockWidth(rowWidth);
  return Math.round(Math.max(DOCK_MIN_WIDTH_PX, Math.min(width, max)));
}

function clamp01(ratio: number): number {
  return Number.isFinite(ratio) ? Math.max(0, Math.min(1, ratio)) : 1;
}

export function dockWidthFromRatio(ratio: number, rowWidth: number): number {
  const max = maxDockWidth(rowWidth);
  return Math.round(DOCK_MIN_WIDTH_PX + clamp01(ratio) * (max - DOCK_MIN_WIDTH_PX));
}

export function dockRatioFromWidth(width: number, rowWidth: number): number {
  const max = maxDockWidth(rowWidth);
  if (max <= DOCK_MIN_WIDTH_PX) return 1;
  return clamp01((clampDockWidth(width, rowWidth) - DOCK_MIN_WIDTH_PX) / (max - DOCK_MIN_WIDTH_PX));
}

export function defaultDockWidth(rowWidth: number): number {
  return clampDockWidth(DOCK_PREFERRED_WIDTH_PX, rowWidth);
}

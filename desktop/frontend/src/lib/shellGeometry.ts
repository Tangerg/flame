// Window-shell geometry — the numbers the drawer and the flank are sized by.
//
// Lives here rather than beside the shell components because both the view layer
// (which clamps a live drag) and the preference store (which persists the
// settled value) need them, and `state` must not import `ui`.

export const SIDEBAR_MIN_WIDTH_PX = 240;
export const SIDEBAR_DEFAULT_WIDTH_PX = 275;
const SIDEBAR_MAX_WIDTH_PX = 520;
const SIDEBAR_READING_MIN_WIDTH_PX = 240;

/** Floor the flank may never cross, whatever the row can spare. */
export const DOCK_MIN_WIDTH_PX = 320;
/** Reserved for the conversation before the flank may claim anything. */
export const DOCK_SAFE_AREA_PX = 352;
/** Preferred measure once the row is wide enough to grant it. */
export const DOCK_PREFERRED_WIDTH_PX = 640;
/** Below this the preferred measure yields to the conversation's own share. */
const DOCK_PREFERRED_SAFE_AREA_PX = 500;
/** A flank sized against the window's height reads as a document, not a strip. */
const DOCK_ASPECT_RATIO = 16 / 10;

/**
 * Whether both work surfaces can coexist. The flank folds through its existing
 * navigation owner below this, rather than compressing either column past the
 * point where it can still be worked in.
 */
export function canPresentDock(rowWidth: number): boolean {
  return rowWidth >= DOCK_MIN_WIDTH_PX + DOCK_SAFE_AREA_PX;
}

export function clampSidebarWidth(width: number, shellWidth: number): number {
  return Math.round(Math.min(maxSidebarWidth(shellWidth), Math.max(SIDEBAR_MIN_WIDTH_PX, width)));
}

/** The Work Index has its own bounded source-list measure; it does not borrow
 *  the Context Dock's reading floor. The live clamp still leaves one
 *  minimum-width reading plane beside it. */
export function maxSidebarWidth(shellWidth: number): number {
  return Math.max(
    SIDEBAR_MIN_WIDTH_PX,
    Math.min(SIDEBAR_MAX_WIDTH_PX, shellWidth - SIDEBAR_READING_MIN_WIDTH_PX),
  );
}

/**
 * Widest the flank may be drawn at this row width.
 *
 * The conversation's safe area is subtracted first, and the floor wins when the
 * row cannot even grant that — a row too narrow for both is already folded.
 *
 * This `Math.max` is also the SOLE owner of the floor, which is why the narrow end
 * of the range is the constant and not a function: a max that can never fall below
 * the floor cannot invert the range, so a second clamp on the other end would have
 * nothing left to do. There was one, and it did nothing — it read as a guard while
 * the guarantee lived entirely here.
 */
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

/**
 * A STORED RATIO, not a stored width.
 *
 * The flank's px measure is the row's business and changes with every window
 * resize; what the person chose is where they put it inside the range that row
 * allows. Keeping the ratio is what lets the same preference read correctly on
 * a laptop and on a 32-inch display, and what lets CSS re-derive the width on
 * resize with no React render at all.
 */
export function dockWidthFromRatio(ratio: number, rowWidth: number): number {
  const max = maxDockWidth(rowWidth);
  return Math.round(DOCK_MIN_WIDTH_PX + clamp01(ratio) * (max - DOCK_MIN_WIDTH_PX));
}

export function dockRatioFromWidth(width: number, rowWidth: number): number {
  const max = maxDockWidth(rowWidth);
  // A row narrower than floor + safe area has no range to sit in — every width in it
  // is the floor, and the position within a zero-width range is a division by zero.
  if (max <= DOCK_MIN_WIDTH_PX) return 1;
  return clamp01((clampDockWidth(width, rowWidth) - DOCK_MIN_WIDTH_PX) / (max - DOCK_MIN_WIDTH_PX));
}

/**
 * Where an unconfigured flank opens.
 *
 * Three claims, and the widest that all of them allow wins: the floor, a
 * measure proportioned to the window's own height, and the preferred width.
 * The height term is what stops a tall narrow window opening a flank that
 * reaches nearly to the top of the conversation.
 */
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

export const NOTIFICATION_TOP_PROPERTY = "--notification-top";
export const NOTIFICATION_BOTTOM_PROPERTY = "--notification-bottom";

interface VerticalSpan {
  top: number;
  bottom: number;
}

interface NotificationInsets {
  top: number;
  bottom: number;
}

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}

/**
 * Viewport insets of the larger free band above or below the composer footprint inside the
 * conversation layout. Measured bounds are clamped to the viewport and layout; above wins ties.
 */
export function notificationInsets(
  layout: VerticalSpan,
  footprint: VerticalSpan,
  viewportHeight: number,
): NotificationInsets {
  const top = clamp(layout.top, 0, viewportHeight);
  const bottom = clamp(layout.bottom, top, viewportHeight);
  const occupied = {
    top: clamp(footprint.top, top, bottom),
    bottom: clamp(footprint.bottom, top, bottom),
  };
  const band =
    occupied.top - top >= bottom - occupied.bottom
      ? { top, bottom: occupied.top }
      : { top: occupied.bottom, bottom };
  return { top: band.top, bottom: viewportHeight - band.bottom };
}

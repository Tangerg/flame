export function readingUnitFor(
  target: Element,
  boundary: Element,
  fits: (unit: Element) => boolean,
): Element {
  // The outermost fitting declared context keeps a whole task's caption and
  // content beside a focused inner group; an option's label is the fallback.
  const units: Element[] = [];
  for (
    let unit = target.closest("[data-focus-unit]");
    unit && unit !== boundary && boundary.contains(unit);
    unit = unit.parentElement?.closest("[data-focus-unit]") ?? null
  )
    units.unshift(unit);
  const label = target.closest("label");
  if (label && label !== boundary && boundary.contains(label))
    units.push(label);
  return units.find(fits) ?? target;
}

export function fitsStickyReadingFrame({
  headerHeight,
  frameHeight,
  bodyLineHeight,
  bodyInset,
}: {
  headerHeight: number;
  frameHeight: number;
  bodyLineHeight: number;
  bodyInset: number;
}): boolean {
  return (
    headerHeight > 0 &&
    headerHeight < frameHeight &&
    headerHeight + bodyInset + 2 * bodyLineHeight <= frameHeight
  );
}

export function isPressedReadingTarget(
  target: Element,
  pressed: Element | null,
): boolean {
  return Boolean(
    pressed &&
    (target.contains(pressed) ||
      pressed.closest("label, [data-focus-proxy]")?.contains(target)),
  );
}

export function readingViewportBounds(viewport: HTMLElement) {
  const bounds = viewport.getBoundingClientRect();
  const style = viewport.ownerDocument.defaultView!.getComputedStyle(viewport);
  return {
    top: bounds.top + (parseFloat(style.scrollPaddingTop) || 0),
    bottom: bounds.bottom - (parseFloat(style.scrollPaddingBottom) || 0),
    width: bounds.width,
  };
}

export function withinReadingFrame(
  bounds: Pick<DOMRect, "top" | "bottom" | "height">,
  frame: { top: number; bottom: number },
): boolean {
  // A rounded scroll offset may differ from native page-zoom geometry by half a CSS pixel.
  return (
    frame.bottom > frame.top &&
    bounds.top >= frame.top - 0.5 &&
    (bounds.height > frame.bottom - frame.top
      ? bounds.top < frame.bottom
      : bounds.bottom <= frame.bottom + 0.5)
  );
}

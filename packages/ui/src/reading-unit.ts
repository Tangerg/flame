export function readingUnitFor(
  target: Element,
  boundary: Element,
  fits: (unit: Element) => boolean,
): Element {
  // A declared group carries its caption; an option's label is the fallback.
  for (const unit of [
    target.closest("[data-focus-unit]"),
    target.closest("label"),
  ]) {
    if (unit && unit !== boundary && boundary.contains(unit) && fits(unit))
      return unit;
  }
  return target;
}

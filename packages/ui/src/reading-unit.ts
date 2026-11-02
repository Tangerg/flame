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

// Hidden, inert and aria-hidden subtrees are not presented to every user, so
// focus must never land in one.
export function isConcealed(element: Element): boolean {
  return element.closest("[hidden], [inert], [aria-hidden='true']") !== null;
}

function availableFocusTarget(
  target: HTMLElement | null | undefined,
): target is HTMLElement {
  if (
    !target?.isConnected ||
    isConcealed(target) ||
    target.matches(":disabled")
  )
    return false;
  const window = target.ownerDocument.defaultView!;
  for (let node: HTMLElement | null = target; node; node = node.parentElement) {
    const style = window.getComputedStyle(node);
    if (
      style.display === "none" ||
      style.visibility === "hidden" ||
      style.visibility === "collapse"
    )
      return false;
  }
  return true;
}

export function handoffFocus(
  source: HTMLElement,
  target: HTMLElement | null | undefined,
): boolean {
  if (
    !source.contains(source.ownerDocument.activeElement) ||
    target?.ownerDocument !== source.ownerDocument ||
    !availableFocusTarget(target)
  )
    return false;
  target.focus({ preventScroll: true });
  return target.ownerDocument.activeElement === target;
}

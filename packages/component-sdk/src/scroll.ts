export type ScrollPosition = { element: HTMLElement; top: number };

export function scrollPositions(element: HTMLElement | null): ScrollPosition[] {
  const positions: ScrollPosition[] = [];
  for (
    let parent = element?.parentElement;
    parent;
    parent = parent.parentElement
  ) {
    if (/(auto|scroll)/.test(getComputedStyle(parent).overflowY))
      positions.push({ element: parent, top: parent.scrollTop });
  }
  return positions;
}

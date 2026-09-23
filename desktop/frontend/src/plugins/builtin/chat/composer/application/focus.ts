let target: HTMLTextAreaElement | null = null;

export function setComposerFocusTarget(element: HTMLTextAreaElement | null): void {
  target = element;
}

export function focusComposer(selectionEnd?: number): void {
  const element = target;
  if (!element) return;
  element.focus();
  if (selectionEnd !== undefined) element.setSelectionRange(selectionEnd, selectionEnd);
}

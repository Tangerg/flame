// Focusing the composer is a CAPABILITY of the composer, never a DOM lookup a caller may
// perform. A module-level handle rather than a threaded ref: there is one composer per
// window and its callers sit on the far side of two context boundaries, which a ref would
// have to travel through — that is how a shortcut once ended up reaching for a class name.

let target: HTMLTextAreaElement | null = null;

/** Called by the composer's own input controller for as long as it is mounted. */
export function setComposerFocusTarget(element: HTMLTextAreaElement | null): void {
  target = element;
}

/**
 * Focus the composer's input. `selectionEnd` collapses the caret there — used
 * when text is loaded back in for editing, so the user continues at the end of
 * what they wrote rather than the start.
 */
export function focusComposer(selectionEnd?: number): void {
  const element = target;
  if (!element) return;
  element.focus();
  if (selectionEnd !== undefined) element.setSelectionRange(selectionEnd, selectionEnd);
}

import type { ClipboardEvent, KeyboardEvent } from "react";
import { isImeKey } from "./keyboard";

const EDITABLE =
  "input, textarea, select, [contenteditable]:not([contenteditable='false'])";

function isEditableTarget(reader: HTMLElement, target: EventTarget): boolean {
  const editable = (target as Element).closest(EDITABLE);
  return editable !== null && reader.contains(editable);
}

// The event owner is the focused reader; `content` is the presented source inside
// it, so framing such as a reader's path label stays outside the selected range.
export function selectAllInReader(
  event: KeyboardEvent<HTMLElement>,
  content: Element = event.currentTarget,
): void {
  if (
    event.defaultPrevented ||
    isImeKey(event.nativeEvent) ||
    event.altKey ||
    event.shiftKey ||
    (!event.ctrlKey && !event.metaKey) ||
    event.key.toLowerCase() !== "a"
  )
    return;
  const reader = event.currentTarget;
  if (!reader.contains(content) || isEditableTarget(reader, event.target))
    return;
  const selection = reader.ownerDocument.getSelection();
  if (!selection) return;
  event.preventDefault();
  selection.selectAllChildren(content);
}

// Select All spans the content's DOM framing (labels, controls, virtual spacers),
// so a whole-content copy writes the reader's presented text instead.
export function copyWholeReader(
  event: ClipboardEvent<HTMLElement>,
  text: string,
  content: Element = event.currentTarget,
): void {
  const reader = event.currentTarget;
  if (
    event.defaultPrevented ||
    !reader.contains(content) ||
    isEditableTarget(reader, event.target) ||
    !isWholeContentSelected(content)
  )
    return;
  event.preventDefault();
  event.clipboardData.setData("text/plain", text);
}

function isWholeContentSelected(content: Element): boolean {
  const selection = content.ownerDocument.getSelection();
  if (selection?.rangeCount !== 1) return false;
  const range = selection.getRangeAt(0);
  return (
    !range.collapsed &&
    range.startContainer === content &&
    range.startOffset === 0 &&
    range.endContainer === content &&
    range.endOffset === content.childNodes.length
  );
}

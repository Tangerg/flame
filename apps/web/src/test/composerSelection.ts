import { act } from "@testing-library/react";
import {
  $getNearestNodeFromDOMNode,
  $getRoot,
  $isTextNode,
  getNearestEditorFromDOMNode,
} from "lexical";

export async function placeComposerCaret(input: HTMLElement, offset: number) {
  const editor = getNearestEditorFromDOMNode(input);
  if (!editor) throw new Error("composer editor is unavailable");
  await act(async () =>
    editor.update(
      () => {
        const document = input.ownerDocument;
        const walker = document.createTreeWalker(input, NodeFilter.SHOW_TEXT);
        let remaining = offset;
        for (let node = walker.nextNode(); node; node = walker.nextNode()) {
          if (remaining <= node.textContent!.length) {
            const text = $getNearestNodeFromDOMNode(node);
            if (!$isTextNode(text)) throw new Error("composer caret must address editor text");
            text.select(remaining, remaining);
            return;
          }
          remaining -= node.textContent!.length;
        }
        if (offset !== 0) throw new Error("composer caret is outside the captured text");
        $getRoot().selectStart();
      },
      { discrete: true },
    ),
  );
}

export function composerCaret(input: HTMLElement): readonly [number, number] {
  const selection = input.ownerDocument.getSelection()!;
  const offset = (node: Node | null, end: number) => {
    if (!node || !input.contains(node)) return -1;
    const range = input.ownerDocument.createRange();
    range.selectNodeContents(input);
    range.setEnd(node, end);
    return range.toString().length;
  };
  return [
    offset(selection.anchorNode, selection.anchorOffset),
    offset(selection.focusNode, selection.focusOffset),
  ];
}

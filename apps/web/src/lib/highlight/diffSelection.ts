import type { ClipboardEvent } from "react";
import type { FileDiffMetadata } from "@pierre/diffs";

function sourceLine(file: FileDiffMetadata, line: Element): string {
  const number = Number(line.getAttribute("data-line"));
  const deleted =
    line.closest("code[data-deletions]") !== null ||
    line.getAttribute("data-line-type")?.endsWith("deletion") === true;
  const start = deleted ? "deletionStart" : "additionStart";
  const count = deleted ? "deletionCount" : "additionCount";
  const index = deleted ? "deletionLineIndex" : "additionLineIndex";
  const hunk = file.hunks.find(
    (item) => number >= item[start] && number < item[start] + item[count],
  );
  const source = deleted ? file.deletionLines : file.additionLines;
  const text = hunk ? source[hunk[index] + number - hunk[start]] : undefined;
  if (text === undefined) throw new Error("workspace diff: rendered line has no captured source");
  return text;
}

// WebKit can report a collapsed document selection for a range inside the diff.
export function selectedDiffRange(root: ShadowRoot): Range | null {
  const document = root.ownerDocument;
  const selection = document.getSelection();
  const selected = selection?.getComposedRanges({ shadowRoots: [root] })[0];
  if (!selected || !root.contains(selected.startContainer) || !root.contains(selected.endContainer))
    return null;
  const range = document.createRange();
  range.setStart(selected.startContainer, selected.startOffset);
  range.setEnd(selected.endContainer, selected.endOffset);
  return range.collapsed ? null : range;
}

// Selection remains native; the clipboard projection excludes gutters and EOF labels.
export function copyDiffSelection(
  event: ClipboardEvent<HTMLElement>,
  root: ShadowRoot,
  file: FileDiffMetadata,
) {
  if (event.defaultPrevented) return;
  const range = selectedDiffRange(root);
  if (!range) return;
  const document = root.ownerDocument;
  const parts: string[] = [];
  for (const line of root.querySelectorAll("[data-code] [data-line]")) {
    if (!range.intersectsNode(line)) continue;
    const slice = document.createRange();
    slice.selectNodeContents(line);
    const beyondLine = range.compareBoundaryPoints(Range.END_TO_END, slice) > 0;
    if (range.compareBoundaryPoints(Range.START_TO_START, slice) > 0)
      slice.setStart(range.startContainer, range.startOffset);
    if (range.compareBoundaryPoints(Range.END_TO_END, slice) < 0)
      slice.setEnd(range.endContainer, range.endOffset);
    const prefix = document.createRange();
    prefix.selectNodeContents(line);
    prefix.setEnd(slice.startContainer, slice.startOffset);
    const offset = prefix.toString().length;
    const text = sourceLine(file, line);
    const previous = parts.at(-1);
    // Opposite EOF sides remain separate displayed rows even without source newlines.
    if (previous !== undefined && !previous.endsWith("\n")) parts.push("\n");
    parts.push(text.slice(offset, beyondLine ? text.length : offset + slice.toString().length));
  }
  const text = parts.join("");
  if (text === "") return;
  event.preventDefault();
  event.clipboardData.setData("text/plain", text);
}

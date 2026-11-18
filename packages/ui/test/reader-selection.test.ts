import { afterEach, describe, expect, it, vi } from "vitest";
import type { ClipboardEvent } from "react";
import {
  copyReaderSelection,
  readerSelectionLines,
} from "../src/reader-selection";
afterEach(() => {
  document.getSelection()?.removeAllRanges();
  document.body.replaceChildren();
});

function reader(lines: Array<{ index: number; parts: string[] }>) {
  const viewport = document.createElement("div");
  const content = document.createElement("div");
  content.dataset.readerSource = "";
  viewport.append(content);
  const rows = lines.map(({ index, parts }) => {
    const row = document.createElement("div");
    row.dataset.readerLine = String(index);
    for (const text of parts) {
      const span = document.createElement("span");
      span.textContent = text;
      row.append(span);
    }
    content.append(row);
    return row;
  });
  document.body.append(viewport);
  return { viewport, content, rows };
}

function copy(viewport: HTMLElement, text: string) {
  const preventDefault = vi.fn();
  const setData = vi.fn();
  copyReaderSelection(
    {
      currentTarget: viewport,
      target: viewport,
      defaultPrevented: false,
      preventDefault,
      clipboardData: { setData },
    } as unknown as ClipboardEvent<HTMLElement>,
    text,
  );
  return { preventDefault, setData };
}

describe("source reader selection", () => {
  it("serializes a cross-window partial range through styled UTF-16 endpoints", () => {
    const source = "alpha中😀omega\n\nnot mounted\nlast line\n";
    const { viewport, rows } = reader([
      { index: 0, parts: ["alpha", "中😀", "omega"] },
      { index: 3, parts: ["last line"] },
    ]);
    const range = document.createRange();
    range.setStart(rows[0]!.children[1]!.firstChild!, 1);
    range.setEnd(rows[1]!.firstChild!.firstChild!, 4);
    document.getSelection()!.addRange(range);
    expect(readerSelectionLines(viewport)).toEqual([0, 3]);
    expect(copy(viewport, source).setData).toHaveBeenCalledExactlyOnceWith(
      "text/plain",
      "😀omega\n\nnot mounted\nlast",
    );
    expect(document.getSelection()!.getRangeAt(0)).toBe(range);
    expect(viewport.querySelectorAll("[data-reader-line]")).toHaveLength(2);
  });

  it("maps blank lines and reverse selection without adding placeholder spaces", () => {
    const { viewport, rows } = reader([
      { index: 1, parts: [] },
      { index: 3, parts: ["end😀"] },
    ]);
    document
      .getSelection()!
      .setBaseAndExtent(rows[1]!.firstChild!.firstChild!, 5, rows[0]!, 0);
    expect(
      copy(viewport, "first\n\nmiddle\nend😀\n").setData,
    ).toHaveBeenCalledExactlyOnceWith("text/plain", "\nmiddle\nend😀");
  });

  it("copies the exact whole source including both terminal newlines", () => {
    const { viewport } = reader([{ index: 0, parts: ["first"] }]);
    document.getSelection()!.selectAllChildren(viewport);
    expect(copy(viewport, "first\n\n").setData).toHaveBeenCalledExactlyOnceWith(
      "text/plain",
      "first\n\n",
    );
  });
});

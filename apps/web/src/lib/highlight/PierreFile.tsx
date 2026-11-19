import * as stylex from "@stylexjs/stylex";
import { useMemo, useRef, type ClipboardEvent, type RefObject } from "react";
import { File, useWorkerPool, type FileOptions } from "@pierre/diffs/react";
import { registerCustomCSSVariableTheme, type FileContents } from "@pierre/diffs";
import { useScheme } from "@/lib/appearance";
import { clipboardCodeElement } from "@/lib/clipboard";
import {
  codeSyntaxColors,
  markQuoteLine,
  markReaderLine,
  selectAllInReader,
  type CodeSourceProps,
} from "@flame/ui";

registerCustomCSSVariableTheme("flame-code", codeSyntaxColors);
export const CODE_RENDERING = { preferredHighlighter: "shiki-js" as const, theme: "flame-code" };

const FILE_TARGET_LINE = "data-file-target-line";

const styles = stylex.create({
  // Let CodeBody scroll the full width of unwrapped source.
  readerScroll: {
    inlineSize: "max-content",
    minInlineSize: "100%",
    "--diffs-overflow-override": "visible",
  },
});

export function PierreFile({
  presentation = "standalone",
  source,
  language,
  path = "source",
  lineRange,
  targetLine,
  intent,
  wrapped = false,
  selectionContentRef,
  sourceScrollRef,
}: {
  presentation?: "code-block" | "standalone";
  source: string;
  language?: string;
  path?: string;
  lineRange?: { start: number; end: number };
  targetLine?: number;
  intent?: object | null;
  wrapped?: boolean;
  selectionContentRef?: CodeSourceProps["selectionContentRef"];
  sourceScrollRef?: RefObject<HTMLElement | null>;
}) {
  const isCodeBlock = presentation === "code-block";
  const startLine = lineRange?.start ?? 1;
  const endLine = lineRange?.end;
  const scheme = useScheme();
  const pool = useWorkerPool();
  if (!pool) throw new Error("code rendering requires its shared worker pool");
  const file = useMemo<FileContents>(
    () => ({
      name: path,
      contents: source,
      lang: language,
      cacheKey: JSON.stringify([path, language, source]),
    }),
    [path, language, source],
  );
  const root = useRef<ShadowRoot | null>(null);
  const presentedIntent = useRef<{ file: FileContents; intent: object } | null>(null);
  const options = useMemo<FileOptions<undefined, undefined>>(
    () => ({
      ...CODE_RENDERING,
      themeType: scheme,
      overflow: wrapped ? "wrap" : "scroll",
      disableFileHeader: true,
      disableLineNumbers: endLine === undefined,
      onPostRender: (node, _instance, phase) => {
        if (phase === "unmount") {
          root.current = null;
          if (selectionContentRef) selectionContentRef.current = null;
          if (sourceScrollRef) sourceScrollRef.current = null;
          return;
        }
        const shadow = node.shadowRoot;
        if (!shadow) throw new Error("code renderer omitted its source root");
        root.current = shadow;
        const code = shadow.querySelector<HTMLElement>("[data-file] > [data-code]");
        if (code && endLine !== undefined) {
          code.style.setProperty("--diffs-min-number-column-width", `${String(endLine).length}ch`);
          // Only the displayed text is absolute. Pierre's attributes still address local rows.
          for (const label of code.querySelectorAll(
            "[data-gutter] [data-column-number] > [data-line-number-content]",
          )) {
            const number =
              Number(label.parentElement?.getAttribute("data-column-number")) + startLine - 1;
            const text = number <= endLine ? String(number) : "";
            if (label.textContent !== text) label.textContent = text;
          }
        }
        for (const line of shadow.querySelectorAll<HTMLElement>("[data-code] [data-line]")) {
          const index = Number(line.dataset.line) - 1;
          const number = startLine + index;
          const served = endLine === undefined || number <= endLine;
          if (served) {
            markQuoteLine(line, number);
            markReaderLine(line, index);
          } else {
            line.removeAttribute("data-quote-line");
            line.removeAttribute("data-reader-line");
          }
          if (served && number === targetLine && intent) {
            // Style only this row so Pierre keeps its ordinary source layout.
            line.setAttribute(FILE_TARGET_LINE, "");
            line.style.backgroundColor = "var(--wash-selected)";
            line.style.boxShadow = "inset 2px 0 0 var(--color-accent-text)";
            if (
              presentedIntent.current?.file !== file ||
              presentedIntent.current.intent !== intent
            ) {
              // Vertical placement must not pan source behind the sticky gutter.
              const left = code?.scrollLeft ?? 0;
              line.scrollIntoView({ block: "center" });
              if (code) code.scrollLeft = left;
              presentedIntent.current = { file, intent };
            }
          } else if (line.hasAttribute(FILE_TARGET_LINE)) {
            line.removeAttribute(FILE_TARGET_LINE);
            line.style.removeProperty("background-color");
            line.style.removeProperty("box-shadow");
          }
        }
        if (selectionContentRef) {
          selectionContentRef.current =
            code && endLine !== undefined ? code.querySelector(":scope > [data-content]") : code;
        }
        if (sourceScrollRef) sourceScrollRef.current = code;
      },
    }),
    [
      scheme,
      wrapped,
      startLine,
      endLine,
      targetLine,
      intent,
      file,
      selectionContentRef,
      sourceScrollRef,
    ],
  );
  const copy = (event: ClipboardEvent<HTMLElement>) => {
    const shadow = root.current;
    if (!shadow || event.defaultPrevented) return;
    const selected = shadow.ownerDocument
      .getSelection()
      ?.getComposedRanges({ shadowRoots: [shadow] })[0];
    if (
      !selected ||
      !shadow.contains(selected.startContainer) ||
      !shadow.contains(selected.endContainer)
    )
      return;
    const range = shadow.ownerDocument.createRange();
    range.setStart(selected.startContainer, selected.startOffset);
    range.setEnd(selected.endContainer, selected.endOffset);
    if (range.collapsed) return;
    const content = selectionContentRef?.current;
    if (
      content &&
      range.startContainer === content &&
      range.startOffset === 0 &&
      range.endContainer === content &&
      range.endOffset === content.childNodes.length
    ) {
      event.preventDefault();
      if (isCodeBlock)
        event.clipboardData.setData(
          "text/html",
          clipboardCodeElement(shadow.ownerDocument, source).outerHTML,
        );
      event.clipboardData.setData("text/plain", source);
      return;
    }
    const sourceLines: string[] = source.match(/[^\n]*\n|[^\n]+$/g) ?? [];
    // A served range can end on an empty authored line.
    if (endLine !== undefined && (source === "" || source.endsWith("\n"))) sourceLines.push("");
    const parts: string[] = [];
    for (const line of shadow.querySelectorAll<HTMLElement>("[data-code] [data-line]")) {
      const index = Number(line.dataset.line) - 1;
      if ((endLine !== undefined && startLine + index > endLine) || !range.intersectsNode(line))
        continue;
      const slice = shadow.ownerDocument.createRange();
      slice.selectNodeContents(line);
      const beyond = range.compareBoundaryPoints(Range.END_TO_END, slice) > 0;
      if (range.compareBoundaryPoints(Range.START_TO_START, slice) > 0)
        slice.setStart(range.startContainer, range.startOffset);
      if (range.compareBoundaryPoints(Range.END_TO_END, slice) < 0)
        slice.setEnd(range.endContainer, range.endOffset);
      const prefix = shadow.ownerDocument.createRange();
      prefix.selectNodeContents(line);
      prefix.setEnd(slice.startContainer, slice.startOffset);
      const text = sourceLines[index];
      if (text === undefined) throw new Error("code selection has no captured source line");
      const offset = prefix.toString().length;
      parts.push(text.slice(offset, beyond ? text.length : offset + slice.toString().length));
    }
    if (!parts.length) return;
    event.preventDefault();
    event.clipboardData.setData("text/plain", parts.join(""));
  };
  return (
    // Shadow-root selection needs a select-all boundary inside the host reader's tab stop.
    <div
      role="group"
      {...stylex.props(isCodeBlock && !wrapped && styles.readerScroll)}
      onCopy={copy}
      onKeyDown={(event) => {
        const content = selectionContentRef?.current ?? root.current;
        if (content) selectAllInReader(event, content);
      }}
    >
      <File file={file} options={options} />
    </div>
  );
}

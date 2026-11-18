import { useMemo, useRef, type ClipboardEvent } from "react";
import { File, useWorkerPool, type FileOptions } from "@pierre/diffs/react";
import { registerCustomCSSVariableTheme, type FileContents } from "@pierre/diffs";
import { useScheme } from "@/lib/appearance";
import { codeSyntaxColors, selectAllInReader } from "@flame/ui";

registerCustomCSSVariableTheme("flame-code", codeSyntaxColors);
export const CODE_RENDERING = { preferredHighlighter: "shiki-js" as const, theme: "flame-code" };
export function PierreFile({
  source,
  language,
  path = "source",
  startLine = 1,
  targetLine,
  intent,
  onPresented,
  wrapped = false,
}: {
  source: string;
  language?: string;
  path?: string;
  startLine?: number;
  targetLine?: number;
  intent?: object | null;
  onPresented?(root: HTMLElement): void;
  wrapped?: boolean;
}) {
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
      disableLineNumbers: true,
      onPostRender: (node, _instance, phase) => {
        if (phase === "unmount") {
          root.current = null;
          return;
        }
        const shadow = node.shadowRoot;
        if (!shadow) throw new Error("code renderer omitted its source root");
        root.current = shadow;
        for (const line of shadow.querySelectorAll<HTMLElement>("[data-code] [data-line]")) {
          const number = Number(line.dataset.line) + startLine - 1;
          line.dataset.quoteLine = String(number);
          line.dataset.readerLine = String(number - startLine);
          if (
            number === targetLine &&
            intent &&
            (presentedIntent.current?.file !== file || presentedIntent.current.intent !== intent)
          ) {
            line.scrollIntoView({ block: "center" });
            presentedIntent.current = { file, intent };
          }
        }
        onPresented?.(node);
      },
    }),
    [scheme, wrapped, startLine, targetLine, intent, file, onPresented],
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
    const sourceLines = source.match(/[^\n]*\n|[^\n]+$/g) ?? [];
    const parts: string[] = [];
    for (const line of shadow.querySelectorAll<HTMLElement>("[data-code] [data-line]")) {
      if (!range.intersectsNode(line)) continue;
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
      const text = sourceLines[Number(line.dataset.line) - 1];
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
      onCopy={copy}
      onKeyDown={(event) => {
        if (root.current) selectAllInReader(event, root.current);
      }}
    >
      <File file={file} options={options} />
    </div>
  );
}

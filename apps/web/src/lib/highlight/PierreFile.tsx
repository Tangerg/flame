import { useEffect, useMemo, useRef, useState, type ClipboardEvent } from "react";
import { File, WorkerPoolContext, type FileOptions } from "@pierre/diffs/react";
import { registerCustomCSSVariableTheme, type FileContents } from "@pierre/diffs";
import { WorkerPoolManager } from "@pierre/diffs/worker";
import workerSource from "@pierre/diffs/worker/worker-portable.js?raw";
import { useScheme } from "@/lib/appearance";
import { codeSyntaxColors, selectAllInReader } from "@flame/ui";

registerCustomCSSVariableTheme("flame-code", codeSyntaxColors);
const rendering = { preferredHighlighter: "shiki-js" as const, theme: "flame-code" };
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
  const file = useMemo<FileContents>(
    () => ({
      name: path,
      contents: source,
      lang: language,
      cacheKey: JSON.stringify([path, language, source]),
    }),
    [path, language, source],
  );
  const [prepared, setPrepared] = useState<
    { file: FileContents; pool: WorkerPoolManager } | { file: FileContents; error: unknown }
  >();
  const root = useRef<ShadowRoot | null>(null);
  const presentedIntent = useRef<{ file: FileContents; intent: object } | null>(null);
  useEffect(() => {
    const pool = new WorkerPoolManager(
      {
        poolSize: 1,
        totalASTLRUCacheSize: 1,
        workerFactory: () => {
          const url = URL.createObjectURL(new Blob([workerSource], { type: "text/javascript" }));
          try {
            return new Worker(url, { type: "module" });
          } finally {
            URL.revokeObjectURL(url);
          }
        },
      },
      rendering,
    );
    let active = true;
    void pool.primeFileHighlightCache(file).then(
      () => {
        if (active) setPrepared({ file, pool });
      },
      (error: unknown) => {
        if (active) setPrepared({ file, error });
      },
    );
    return () => {
      active = false;
      pool.terminate();
    };
  }, [file]);
  const options = useMemo<FileOptions<undefined, undefined>>(
    () => ({
      ...rendering,
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
  if (!prepared || prepared.file !== file) return <span role="status" aria-busy="true" />;
  if ("error" in prepared) throw prepared.error;
  return (
    // Shadow-root selection needs a select-all boundary inside the host reader's tab stop.
    // oxlint-disable-next-line jsx-a11y/no-noninteractive-element-interactions
    <div
      role="group"
      onCopy={copy}
      onKeyDown={(event) => {
        if (root.current) selectAllInReader(event, root.current);
      }}
    >
      <WorkerPoolContext value={prepared.pool}>
        <File file={file} options={options} />
      </WorkerPoolContext>
    </div>
  );
}

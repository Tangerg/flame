import { useCallback, useLayoutEffect, useMemo, useRef, useState } from "react";
import { Button, TextField } from "@flame/ui";
import { ansiText, renderAnsi } from "./ansi";

const PREVIEW_LINES = 9;
const OVERSCAN = 6;

export function RecordedOutput({ text }: { text: string }) {
  const lines = useMemo(() => text.replace(/\n$/, "").split("\n"), [text]);
  const [expanded, setExpanded] = useState(false);
  const [finding, setFinding] = useState(false);
  const [query, setQuery] = useState("");
  const [match, setMatch] = useState(0);
  const [metrics, setMetrics] = useState({ height: 0, top: 0, row: 1 });
  const viewport = useRef<HTMLDivElement>(null);
  const ruler = useRef<HTMLSpanElement>(null);
  const search = useRef<HTMLElement>(null);
  const matches = useMemo(() => {
    if (!query) return [];
    const needle = query.toLocaleLowerCase();
    return lines.flatMap((line, index) =>
      ansiText(line).toLocaleLowerCase().includes(needle) ? [index] : [],
    );
  }, [lines, query]);
  const selected = matches.length ? matches[match % matches.length] : undefined;
  const atLatest =
    lines.length * metrics.row - metrics.top - metrics.height < 1;

  const readWindow = useCallback(() => {
    const node = viewport.current;
    if (!node) return;
    setMetrics({
      height: node.clientHeight,
      top: node.scrollTop,
      row: parseFloat(getComputedStyle(node).lineHeight),
    });
  }, []);

  const jumpToLatest = useCallback(() => {
    const node = viewport.current;
    if (!node) return;
    node.scrollTop = node.scrollHeight;
    readWindow();
  }, [readWindow]);

  useLayoutEffect(() => {
    const node = viewport.current;
    if (!expanded || !node) return;
    readWindow();
    const observer = new ResizeObserver(readWindow);
    observer.observe(node);
    // The fixed viewport does not resize when the code font changes.
    observer.observe(ruler.current!);
    return () => observer.disconnect();
  }, [expanded, readWindow]);

  useLayoutEffect(() => {
    if (!expanded) return;
    const frame = requestAnimationFrame(jumpToLatest);
    return () => cancelAnimationFrame(frame);
  }, [expanded, lines, jumpToLatest]);

  useLayoutEffect(() => {
    if (finding) search.current?.focus();
  }, [finding]);

  useLayoutEffect(() => {
    const node = viewport.current;
    if (!node || selected === undefined) return;
    node.scrollTop = Math.max(
      0,
      selected * metrics.row - node.clientHeight / 2,
    );
    readWindow();
  }, [selected, metrics.row, readWindow]);

  function closeFind() {
    setFinding(false);
    setQuery("");
    viewport.current?.focus();
  }

  const start = expanded
    ? Math.max(0, Math.floor(metrics.top / metrics.row) - OVERSCAN)
    : Math.max(0, lines.length - PREVIEW_LINES);
  const end = expanded
    ? Math.min(
        lines.length,
        Math.ceil((metrics.top + metrics.height) / metrics.row) + OVERSCAN,
      )
    : lines.length;
  const visible = lines.slice(start, end).map((line, offset) => (
    <div
      key={start + offset}
      style={{ height: expanded ? metrics.row : undefined, whiteSpace: "pre" }}
    >
      <span
        data-output-line={start + offset}
        data-current-match={start + offset === selected ? "" : undefined}
      >
        {renderAnsi(line || " ")}
      </span>
    </div>
  ));

  return (
    <div
      style={{
        minWidth: 0,
        position: "relative",
        fontFamily: "var(--font-mono)",
        fontSize: "var(--fs-code)",
        lineHeight: "var(--leading-relaxed)",
      }}
    >
      <span
        ref={ruler}
        aria-hidden="true"
        style={{
          position: "absolute",
          visibility: "hidden",
          height: "1lh",
          width: 0,
        }}
      />
      {expanded ? (
        <>
          {finding && (
            <div role="search" aria-label="Output search">
              <TextField
                ref={search}
                type="search"
                aria-label="Find in output"
                value={query}
                onChange={(event) => {
                  setQuery(event.target.value);
                  setMatch(0);
                }}
                onKeyDown={(event) => {
                  if (event.key === "Escape") {
                    event.preventDefault();
                    closeFind();
                  }
                  if (event.key === "Enter" && matches.length) {
                    event.preventDefault();
                    setMatch(
                      (current) =>
                        (current + (event.shiftKey ? matches.length - 1 : 1)) %
                        matches.length,
                    );
                  }
                }}
              />
              <span role="status">
                {matches.length
                  ? `${(match % matches.length) + 1} of ${matches.length}`
                  : "No matches"}
              </span>
            </div>
          )}
          <div
            ref={viewport}
            role="region"
            aria-label="Tool output"
            tabIndex={0}
            data-focus-inset=""
            style={{
              height: `min(50vh,24rem,${lines.length}lh)`,
              minWidth: 0,
              overflow: "auto",
              overflowAnchor: "none",
              overscrollBehavior: "contain",
            }}
            onScroll={readWindow}
            onKeyDown={(event) => {
              if (
                (event.metaKey || event.ctrlKey) &&
                event.key.toLowerCase() === "f"
              ) {
                event.preventDefault();
                setFinding(true);
              }
            }}
          >
            <div style={{ height: start * metrics.row }} />
            {visible}
            <div style={{ height: (lines.length - end) * metrics.row }} />
          </div>
          {!atLatest && <Button onClick={jumpToLatest}>Jump to latest</Button>}
          <Button
            onClick={() => {
              setExpanded(false);
              setFinding(false);
              setQuery("");
            }}
          >
            Collapse
          </Button>
        </>
      ) : (
        <>
          {lines.length > PREVIEW_LINES && (
            <Button onClick={() => setExpanded(true)}>
              Show {lines.length - PREVIEW_LINES} earlier lines · {lines.length}{" "}
              total
            </Button>
          )}
          <div
            role="region"
            aria-label="Tool output preview"
            tabIndex={0}
            data-focus-inset=""
            style={{ overflowX: "auto" }}
          >
            {visible}
          </div>
        </>
      )}
    </div>
  );
}

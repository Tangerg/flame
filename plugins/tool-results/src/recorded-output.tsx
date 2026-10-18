import {
  useCallback,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent,
  type ReactNode,
  type RefObject,
} from "react";
import { ArrowDownToLine, ChevronUp, Search, X } from "@flame/icons/react";
import { Button, IconButton, SearchField } from "@flame/ui";
import { ansiText, renderAnsi } from "./ansi";

const PREVIEW_LINES = 9;
const OVERSCAN = 6;

export const controlStrip: CSSProperties = {
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: "var(--layout-control-gap)",
  minWidth: 0,
  marginBlockEnd: "var(--layout-label-gap)",
  fontFamily: "var(--font-sans)",
  fontSize: "var(--fs-ui-sm)",
  lineHeight: "var(--leading-body)",
};

export const controlCluster: CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: "2px",
  marginInlineStart: "auto",
};

export function isFindShortcut(event: KeyboardEvent) {
  return (event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "f";
}

export function FindField({
  label,
  fieldLabel,
  value,
  fieldRef,
  status,
  onChange,
  onClose,
  onEnter,
}: {
  label: string;
  fieldLabel: string;
  value: string;
  fieldRef: RefObject<HTMLElement | null>;
  status?: ReactNode;
  onChange: (value: string) => void;
  onClose: () => void;
  onEnter?: (backward: boolean) => void;
}) {
  return (
    <div
      role="search"
      aria-label={label}
      style={{
        display: "flex",
        flex: "1 1 14rem",
        alignItems: "center",
        gap: "var(--layout-control-gap)",
        minWidth: 0,
      }}
    >
      <div style={{ flex: 1, minWidth: 0 }}>
        <SearchField
          ref={fieldRef}
          size="sm"
          aria-label={fieldLabel}
          placeholder={fieldLabel}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Escape") {
              event.preventDefault();
              onClose();
            }
            if (event.key === "Enter" && onEnter) {
              event.preventDefault();
              onEnter(event.shiftKey);
            }
          }}
        />
      </div>
      {status}
      <IconButton
        icon={X}
        size="xs"
        quiet
        aria-label="Close find"
        onClick={onClose}
      />
    </div>
  );
}

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
  const expansionOpener = useRef<HTMLButtonElement>(null);
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
    const frame = requestAnimationFrame(() => {
      jumpToLatest();
      const opener = expansionOpener.current;
      expansionOpener.current = null;
      const node = viewport.current;
      if (
        opener &&
        !opener.isConnected &&
        node &&
        node.ownerDocument.activeElement === node.ownerDocument.body
      ) {
        node.focus();
      }
    });
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
  const earlier = lines.length - PREVIEW_LINES;

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
          <div data-slot="output-controls" style={controlStrip}>
            {finding && (
              <FindField
                label="Output search"
                fieldLabel="Find in output"
                value={query}
                fieldRef={search}
                status={
                  query && (
                    <span
                      role="status"
                      style={{
                        flexShrink: 0,
                        color: "var(--color-text-faint)",
                        fontVariantNumeric: "tabular-nums",
                      }}
                    >
                      {matches.length
                        ? `${(match % matches.length) + 1} of ${matches.length}`
                        : "No matches"}
                    </span>
                  )
                }
                onChange={(value) => {
                  setQuery(value);
                  setMatch(0);
                }}
                onClose={closeFind}
                onEnter={(backward) => {
                  if (!matches.length) return;
                  setMatch(
                    (current) =>
                      (current + (backward ? matches.length - 1 : 1)) %
                      matches.length,
                  );
                }}
              />
            )}
            <div style={controlCluster}>
              {!atLatest && (
                <IconButton
                  icon={ArrowDownToLine}
                  size="xs"
                  quiet
                  aria-label="Jump to latest"
                  onClick={() => {
                    jumpToLatest();
                    viewport.current?.focus();
                  }}
                />
              )}
              {!finding && (
                <IconButton
                  icon={Search}
                  size="xs"
                  quiet
                  aria-label="Find"
                  onClick={() => setFinding(true)}
                />
              )}
              <IconButton
                icon={ChevronUp}
                size="xs"
                quiet
                aria-label="Collapse"
                onClick={() => {
                  setExpanded(false);
                  setFinding(false);
                  setQuery("");
                }}
              />
            </div>
          </div>
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
              if (isFindShortcut(event)) {
                event.preventDefault();
                setFinding(true);
                search.current?.focus();
              }
            }}
          >
            <div style={{ height: start * metrics.row }} />
            {visible}
            <div style={{ height: (lines.length - end) * metrics.row }} />
          </div>
        </>
      ) : (
        <>
          {earlier > 0 && (
            <div data-slot="output-controls" style={controlStrip}>
              <Button
                size="xs"
                onClick={(event) => {
                  expansionOpener.current = event.currentTarget;
                  setExpanded(true);
                }}
              >
                Show {earlier} earlier {earlier === 1 ? "line" : "lines"} ·{" "}
                {lines.length} total
              </Button>
            </div>
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

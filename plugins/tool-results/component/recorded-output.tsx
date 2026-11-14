import {
  useCallback,
  useLayoutEffect,
  useMemo,
  useReducer,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent,
  type ReactNode,
  type RefObject,
} from "react";
import { ArrowDownToLine, Search, X } from "@flame/icons/react";
import {
  Button,
  CodeCaption,
  CodeFrame,
  IconButton,
  copyReaderSelection,
  readerSelectionLines,
  useContentVisible,
  isImeKey,
  SearchField,
  selectAllInReader,
  type CodeActionLabels,
} from "@flame/ui";
import { defaultRangeExtractor, useVirtualizer } from "@tanstack/react-virtual";
import { ansiText, renderAnsi } from "./ansi";
import type { Translate } from "@flame/component-sdk";

const PREVIEW_LINES = 9;
const OVERSCAN = 6;

export const frameInset = "calc(var(--spacing) * 3)";

export const findBar: CSSProperties = {
  display: "flex",
  minWidth: 0,
  paddingInline: `${frameInset} calc(var(--spacing) * 1.5)`,
  paddingBlockEnd: "var(--layout-label-gap)",
  fontFamily: "var(--font-sans)",
  fontSize: "var(--fs-ui-sm)",
  lineHeight: "var(--leading-body)",
};

export function resultActions(t: Translate, copy: string): CodeActionLabels {
  return {
    copy,
    copied: t("action.copied"),
    wrap: t("action.wrap"),
    unwrap: t("action.unwrap"),
  };
}

export function isFindShortcut(event: KeyboardEvent) {
  return (
    !event.defaultPrevented &&
    !isImeKey(event.nativeEvent) &&
    (event.metaKey || event.ctrlKey) &&
    event.key.toLowerCase() === "f"
  );
}

// Repeating Find on the already-focused field fires no focus-in, so every
// explicit entry also asks the containing scroller to reveal the field.
export function enterFind(field: HTMLElement | null) {
  if (!field) return;
  field.focus({ preventScroll: true });
  field.scrollIntoView({
    block: "nearest",
    inline: "nearest",
    behavior: "instant",
  });
}

export function FindField({
  t,
  label,
  fieldLabel,
  value,
  fieldRef,
  status,
  onChange,
  onClose,
  onEnter,
}: {
  t: Translate;
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
    // Find is a bounded task: Escape closes it even over a query, so the
    // close runs in capture before the field's own clear-on-Escape.
    <div
      role="search"
      aria-label={label}
      onKeyDownCapture={(event) => {
        if (
          event.key !== "Escape" ||
          event.defaultPrevented ||
          isImeKey(event.nativeEvent)
        )
          return;
        event.preventDefault();
        onClose();
      }}
      onKeyDown={(event) => {
        if (!isFindShortcut(event)) return;
        event.preventDefault();
        event.stopPropagation();
        enterFind(fieldRef.current);
      }}
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
          onClear={() => onChange("")}
          clearLabel={t("find.clear")}
          onKeyDown={(event) => {
            if (
              event.key === "Enter" &&
              onEnter &&
              !isImeKey(event.nativeEvent)
            ) {
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
        aria-label={t("find.close")}
        onClick={onClose}
      />
    </div>
  );
}

export function RecordedOutput({ text, t }: { text: string; t: Translate }) {
  const lines = useMemo(() => text.replace(/\n$/, "").split("\n"), [text]);
  const [expanded, setExpanded] = useState(false);
  const visible = useContentVisible();
  const [finding, setFinding] = useState(false);
  // Each explicit Find action publishes a fresh value, even when it selects
  // the same line, so it can reveal that line again after manual scrolling.
  const [find, setFind] = useState({ query: "", match: 0 });
  const { query, match } = find;
  const [rowHeight, setRowHeight] = useState(20);
  const [selectionVersion, invalidateSelection] = useReducer(
    (version: number) => version + 1,
    0,
  );
  const viewport = useRef<HTMLDivElement>(null);
  const reflow = useRef<{ lines: typeof lines; index: number } | null>(null);
  const ruler = useRef<HTMLSpanElement>(null);
  const search = useRef<HTMLElement>(null);
  const expansion = useRef<{
    origin: HTMLButtonElement;
    activeElement: Element | null;
    source: string;
  } | null>(null);
  const matches = useMemo(() => {
    if (!query) return [];
    const needle = query.toLocaleLowerCase();
    return lines.flatMap((line, index) =>
      ansiText(line).toLocaleLowerCase().includes(needle) ? [index] : [],
    );
  }, [lines, query]);
  const selected = matches.length ? matches[match % matches.length] : undefined;
  const virtualizer = useVirtualizer<HTMLDivElement, HTMLDivElement>({
    count: expanded ? lines.length : 0,
    getScrollElement: () => viewport.current,
    estimateSize: () => rowHeight,
    overscan: OVERSCAN,
    rangeExtractor: useCallback(
      (range) => {
        const endpoints = readerSelectionLines(viewport.current);
        return [
          ...new Set([
            ...defaultRangeExtractor(range),
            ...endpoints.filter((index) => index >= 0 && index < range.count),
          ]),
        ].sort((left, right) => left - right);
      },
      [selectionVersion],
    ),
  });
  const atLatest =
    virtualizer.getTotalSize() -
      (virtualizer.scrollOffset ?? 0) -
      (virtualizer.scrollRect?.height ?? 0) <
    1;
  const jumpToLatest = useCallback(() => {
    virtualizer.scrollToIndex(lines.length - 1, { align: "end" });
  }, [virtualizer, lines.length]);

  useLayoutEffect(() => {
    const node = viewport.current;
    if (!expanded || !visible || !node) return;
    const read = () => {
      const height = parseFloat(
        node.ownerDocument.defaultView!.getComputedStyle(node).lineHeight,
      );
      if (height > 0 && height !== rowHeight) {
        const anchor = virtualizer.getVirtualItemForOffset(
          virtualizer.scrollOffset ?? 0,
        );
        reflow.current = anchor ? { lines, index: anchor.index } : null;
        setRowHeight(height);
      }
    };
    read();
    const observer = new ResizeObserver(read);
    observer.observe(ruler.current!);
    node.ownerDocument.addEventListener("selectionchange", invalidateSelection);
    return () => {
      observer.disconnect();
      node.ownerDocument.removeEventListener(
        "selectionchange",
        invalidateSelection,
      );
    };
  }, [expanded, visible, rowHeight, virtualizer, lines]);

  useLayoutEffect(() => {
    const anchor = reflow.current;
    reflow.current = null;
    virtualizer.measure();
    if (anchor?.lines === lines)
      virtualizer.scrollToIndex(anchor.index, { align: "start" });
  }, [virtualizer, rowHeight, lines]);

  useLayoutEffect(() => {
    const intent = expansion.current;
    if (!expanded || !visible || intent?.source !== text) {
      expansion.current = null;
      virtualizer.cancelScroll();
      return;
    }
    const frame = requestAnimationFrame(() => {
      jumpToLatest();
      expansion.current = null;
      const node = viewport.current;
      if (
        intent &&
        node &&
        (node.ownerDocument.activeElement === intent.origin ||
          node.ownerDocument.activeElement === intent.activeElement)
      ) {
        node.focus();
      }
    });
    return () => cancelAnimationFrame(frame);
  }, [expanded, visible, text, jumpToLatest, virtualizer]);

  useLayoutEffect(() => {
    if (finding) enterFind(search.current);
  }, [finding]);

  useLayoutEffect(() => {
    if (selected !== undefined)
      virtualizer.scrollToIndex(selected, { align: "center" });
  }, [find, selected, virtualizer]);

  function closeFind() {
    setFinding(false);
    setFind({ query: "", match: 0 });
    viewport.current?.focus();
  }

  const previewStart = Math.max(0, lines.length - PREVIEW_LINES);
  const presented = lines.slice(previewStart);
  const row = (line: string, index: number, start?: number) => (
    <div
      key={index}
      data-slot="search-line"
      data-current-match={index === selected ? "" : undefined}
      style={{
        position: start === undefined ? undefined : "absolute",
        top: start === undefined ? undefined : 0,
        left: start === undefined ? undefined : 0,
        transform: start === undefined ? undefined : `translateY(${start}px)`,
        height: "1lh",
        minWidth: "100%",
        width: "max-content",
        paddingInline: frameInset,
        whiteSpace: "pre",
      }}
      data-reader-line={start === undefined ? undefined : index}
    >
      <span data-output-line={index}>{renderAnsi(line)}</span>
    </div>
  );
  const earlier = lines.length - PREVIEW_LINES;
  const copied = useMemo(() => ansiText(text), [text]);

  return (
    <CodeFrame>
      <CodeCaption
        language={t("caption.output")}
        code={copied}
        labels={resultActions(t, t("output.copy"))}
        wrappable={false}
      >
        {earlier > 0 && (
          <Button
            size="xs"
            aria-expanded={expanded}
            onClick={(event) => {
              if (expanded) {
                event.currentTarget.focus({ preventScroll: true });
                expansion.current = null;
                setExpanded(false);
                setFinding(false);
                setFind({ query: "", match: 0 });
              } else {
                expansion.current = {
                  origin: event.currentTarget,
                  activeElement:
                    event.currentTarget.ownerDocument.activeElement,
                  source: text,
                };
                setExpanded(true);
              }
            }}
          >
            {expanded
              ? t("output.collapse")
              : t("output.showEarlier", {
                  count: earlier,
                  total: lines.length,
                })}
          </Button>
        )}
        {expanded && !atLatest && (
          <IconButton
            icon={ArrowDownToLine}
            size="xs"
            quiet
            aria-label={t("output.jumpToLatest")}
            onClick={() => {
              jumpToLatest();
              viewport.current?.focus();
            }}
          />
        )}
        {expanded && !finding && (
          <IconButton
            icon={Search}
            size="xs"
            quiet
            aria-label={t("find.open")}
            onClick={() => setFinding(true)}
          />
        )}
      </CodeCaption>
      {expanded && finding && (
        <div data-slot="output-controls" style={findBar}>
          <FindField
            t={t}
            label={t("output.search")}
            fieldLabel={t("output.field")}
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
                    ? t("output.matchCount", {
                        current: (match % matches.length) + 1,
                        total: matches.length,
                      })
                    : t("output.noMatches")}
                </span>
              )
            }
            onChange={(value) => setFind({ query: value, match: 0 })}
            onClose={closeFind}
            onEnter={(backward) => {
              if (!matches.length) return;
              setFind((current) => ({
                query: current.query,
                match:
                  (current.match + (backward ? matches.length - 1 : 1)) %
                  matches.length,
              }));
            }}
          />
        </div>
      )}
      <div
        dir="ltr"
        style={{
          minWidth: 0,
          position: "relative",
          paddingBlockEnd: frameInset,
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
          <div
            ref={viewport}
            role="region"
            aria-label={t("output.region")}
            tabIndex={0}
            data-focus-inset=""
            style={{
              height: `min(50vh,24rem,${lines.length}lh)`,
              minWidth: 0,
              overflow: "auto",
              overflowAnchor: "none",
              overscrollBehavior: "contain auto",
            }}
            onWheel={() => virtualizer.cancelScroll()}
            onPointerDown={() => virtualizer.cancelScroll()}
            onKeyDown={(event) => {
              if (isFindShortcut(event)) {
                event.preventDefault();
                event.stopPropagation();
                setFinding(true);
                enterFind(search.current);
                return;
              }
              if (!isImeKey(event.nativeEvent)) virtualizer.cancelScroll();
              selectAllInReader(event);
            }}
            onCopy={(event) => copyReaderSelection(event, copied)}
          >
            {/* Select All bounds its Range by this region's child offsets, which
                window reconciliation would shift; one stable child keeps it whole. */}
            <div
              data-reader-source=""
              style={{
                height: virtualizer.getTotalSize(),
                position: "relative",
                minWidth: "100%",
                width: "max-content",
              }}
            >
              {virtualizer
                .getVirtualItems()
                .map((item) => row(lines[item.index]!, item.index, item.start))}
            </div>
          </div>
        ) : (
          <div
            role="region"
            aria-label={t("output.preview")}
            tabIndex={0}
            data-focus-inset=""
            style={{ overflowX: "auto" }}
            onKeyDown={selectAllInReader}
            onCopy={(event) =>
              copyReaderSelection(event, presented.map(ansiText).join("\n"))
            }
          >
            {presented.map((line, offset) => row(line, previewStart + offset))}
          </div>
        )}
      </div>
    </CodeFrame>
  );
}

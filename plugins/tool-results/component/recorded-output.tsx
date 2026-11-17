import {
  useLayoutEffect,
  useCallback,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent,
  type ReactNode,
  type RefObject,
} from "react";
import { ArrowDownToLine, Search, X } from "@flame/icons/react";
import {
  ReadingList,
  type ReadingListHandle,
  Button,
  CodeCaption,
  CodeFrame,
  IconButton,
  copyReaderSelection,
  selectAllInReader,
  isImeKey,
  SearchField,
  type CodeActionLabels,
} from "@flame/ui";
import { ansiLineStyles, ansiText, renderAnsi } from "./ansi";
import type { Translate } from "@flame/component-sdk";

const PREVIEW_LINES = 9;
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
  fieldRef: RefObject<HTMLInputElement | null>;
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
  const lineIndices = useMemo(
    () => lines.map((_line, index) => index),
    [lines],
  );
  const lineStyles = useMemo(() => ansiLineStyles(text), [text]);
  const source = useMemo(() => ansiText(text), [text]);
  const [expanded, setExpanded] = useState(false);
  const [finding, setFinding] = useState(false);
  const [find, setFind] = useState({ query: "", match: 0 });
  const [atEnd, setAtEnd] = useState(true);
  const reader = useRef<ReadingListHandle>(null);
  const viewport = useRef<HTMLDivElement>(null);
  const search = useRef<HTMLInputElement>(null);
  const expansion = useRef<{ origin: HTMLElement; source: string } | null>(
    null,
  );
  const matches = useMemo(
    () =>
      !find.query
        ? []
        : lines.flatMap((line, index) =>
            ansiText(line)
              .toLocaleLowerCase()
              .includes(find.query.toLocaleLowerCase())
              ? [index]
              : [],
          ),
    [lines, find.query],
  );
  const selected = matches.length
    ? matches[find.match % matches.length]
    : undefined;
  useLayoutEffect(() => {
    if (selected !== undefined)
      void reader.current?.scrollToKey({
        key: String(selected),
        animated: false,
        viewPosition: 0.5,
      });
  }, [find, selected]);
  useLayoutEffect(() => {
    const intent = expansion.current;
    if (
      !expanded ||
      !intent ||
      intent.source !== text ||
      intent.origin.ownerDocument.activeElement !== intent.origin
    )
      return;
    expansion.current = null;
    reader.current?.getScrollableNode()?.focus({ preventScroll: true });
  }, [expanded, text]);
  useLayoutEffect(() => {
    if (finding) enterFind(search.current);
  }, [finding]);
  const closeFind = () => {
    setFinding(false);
    setFind({ query: "", match: 0 });
    viewport.current?.focus({ preventScroll: true });
  };
  const previewStart = Math.max(0, lines.length - PREVIEW_LINES);
  const row = useCallback(
    (index: number) => (
      <div
        data-slot="search-line"
        data-reader-line={expanded ? index : index - previewStart}
        data-current-match={selected === index ? "" : undefined}
        style={{
          minHeight: "1lh",
          whiteSpace: "pre",
          minWidth: "100%",
          width: "max-content",
          paddingInline: frameInset,
          backgroundColor:
            selected === index ? "var(--color-highlight)" : undefined,
        }}
      >
        <span data-output-line={index}>
          {renderAnsi(lines[index]!, lineStyles[index]!)}
        </span>
      </div>
    ),
    [expanded, selected, previewStart, lines, lineStyles],
  );
  return (
    <CodeFrame>
      <CodeCaption
        language={t("caption.output")}
        code={source}
        labels={resultActions(t, t("output.copy"))}
        wrappable={false}
      >
        {previewStart > 0 && (
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
                  source: text,
                };
                setExpanded(true);
              }
            }}
          >
            {expanded
              ? t("output.collapse")
              : t("output.showEarlier", {
                  count: previewStart,
                  total: lines.length,
                })}
          </Button>
        )}
        {expanded && !atEnd && (
          <IconButton
            icon={ArrowDownToLine}
            size="xs"
            aria-label={t("output.jumpToLatest")}
            onClick={() => {
              void reader.current?.scrollToEnd({ animated: false });
              reader.current
                ?.getScrollableNode()
                ?.focus({ preventScroll: true });
            }}
          />
        )}
        {expanded && !finding && (
          <IconButton
            icon={Search}
            size="xs"
            aria-label={t("find.open")}
            onClick={() => setFinding(true)}
          />
        )}
      </CodeCaption>
      {finding && (
        <div data-slot="output-controls" style={findBar}>
          <FindField
            t={t}
            label={t("output.search")}
            fieldLabel={t("output.field")}
            value={find.query}
            fieldRef={search}
            status={
              find.query && (
                <span role="status">
                  {matches.length
                    ? t("output.matchCount", {
                        current: (find.match % matches.length) + 1,
                        total: matches.length,
                      })
                    : t("output.noMatches")}
                </span>
              )
            }
            onChange={(query) => setFind({ query, match: 0 })}
            onClose={closeFind}
            onEnter={(backward) => {
              if (matches.length)
                setFind(({ query, match }) => ({
                  query,
                  match:
                    (match + (backward ? matches.length - 1 : 1)) %
                    matches.length,
                }));
            }}
          />
        </div>
      )}
      <div
        ref={viewport}
        role="region"
        aria-label={t(expanded ? "output.region" : "output.preview")}
        tabIndex={expanded ? -1 : 0}
        data-reader-source={expanded ? undefined : ""}
        data-quote-source="tool-output"
        onKeyDown={(event) => {
          if (viewport.current) selectAllInReader(event, viewport.current);
          if (expanded && isFindShortcut(event)) {
            event.preventDefault();
            event.stopPropagation();
            setFinding(true);
            enterFind(search.current);
          }
        }}
        onCopy={(event) =>
          copyReaderSelection(
            event,
            expanded
              ? source
              : lines.slice(previewStart).map(ansiText).join("\n"),
          )
        }
        style={{
          fontFamily: "var(--font-mono)",
          fontSize: "var(--fs-code)",
          lineHeight: "var(--leading-relaxed)",
          minHeight: 0,
          display: "flex",
          flexDirection: "column",
          height: expanded ? "min(24rem, 60vh)" : undefined,
        }}
      >
        {expanded ? (
          <ReadingList
            role="group"
            ref={reader}
            data={lineIndices}
            keyExtractor={String}
            label={t("output.region")}
            onFollowChange={(list, value) => {
              setAtEnd(value);
              const node = list.getScrollableNode();
              if (node) {
                node.tabIndex = 0;
                node.setAttribute("data-reader-source", "");
              }
            }}
            renderItem={row}
          />
        ) : (
          lines
            .slice(previewStart)
            .map((_line, offset) => (
              <div key={previewStart + offset}>
                {row(previewStart + offset)}
              </div>
            ))
        )}
      </div>
    </CodeFrame>
  );
}

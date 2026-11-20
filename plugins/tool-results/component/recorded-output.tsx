import { useLayoutEffect, useCallback, useMemo, useRef, useState } from "react";
import { ArrowDownToLine, Search } from "@flame/icons/react";
import {
  ReadingList,
  type ReadingListHandle,
  Button,
  CodeCaption,
  CodeFrame,
  IconButton,
  copyReaderSelection,
  isImeKey,
  markReaderSource,
  quoteSourceProps,
  readerLineProps,
  readerSourceProps,
  scrollWithHorizontalArrow,
  selectAllInReader,
} from "@flame/ui";
import type { Translate } from "@flame/component-sdk";
import { ansiLineStyles, ansiText, renderAnsi } from "./ansi";
import {
  containsQuery,
  enterFind,
  findBar,
  FindField,
  isFindShortcut,
} from "./find";
import { frameInset, resultActions } from "./frame";

const TOOL_OUTPUT_QUOTE_SOURCE = quoteSourceProps({ kind: "tool-output" });

const PREVIEW_LINES = 9;

// Find exists only inside the expanded reader, and its query lives only while
// Find is open; one closed view keeps collapse and close from leaving either.
type OutputView =
  | { mode: "preview" }
  | { mode: "reading" }
  | { mode: "finding"; query: string; match: number };

const PREVIEW: OutputView = { mode: "preview" };
const READING: OutputView = { mode: "reading" };
const FINDING: OutputView = { mode: "finding", query: "", match: 0 };

function openFind(view: OutputView): OutputView {
  return view.mode === "finding" ? view : FINDING;
}

export function RecordedOutput({ text, t }: { text: string; t: Translate }) {
  const lines = useMemo(() => text.replace(/\n$/, "").split("\n"), [text]);
  const lineIndices = useMemo(
    () => lines.map((_line, index) => index),
    [lines],
  );
  const lineStyles = useMemo(() => ansiLineStyles(text), [text]);
  const source = useMemo(() => ansiText(text), [text]);
  const [view, setView] = useState<OutputView>(PREVIEW);
  const expanded = view.mode !== "preview";
  const finding = view.mode === "finding";
  const query = finding ? view.query : "";
  const [atEnd, setAtEnd] = useState(true);
  const reader = useRef<ReadingListHandle>(null);
  const search = useRef<HTMLInputElement>(null);
  const expansion = useRef<{ origin: HTMLElement; source: string } | null>(
    null,
  );
  const matches = useMemo(
    () =>
      !query
        ? []
        : lines.flatMap((line, index) =>
            containsQuery(ansiText(line), query) ? [index] : [],
          ),
    [lines, query],
  );
  const selected =
    finding && matches.length
      ? matches[view.match % matches.length]
      : undefined;
  useLayoutEffect(() => {
    if (selected !== undefined)
      void reader.current?.scrollToKey({
        key: String(selected),
        animated: false,
        viewPosition: 0.5,
      });
  }, [view, selected]);
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
    setView(READING);
    reader.current?.getScrollableNode()?.focus({ preventScroll: true });
  };
  const previewStart = Math.max(0, lines.length - PREVIEW_LINES);
  const row = useCallback(
    (index: number) => (
      <div
        data-slot="search-line"
        {...readerLineProps(expanded ? index : index - previewStart)}
        data-current-match={selected === index ? "" : undefined}
        style={{
          minHeight: "1lh",
          whiteSpace: "pre",
          minWidth: "100%",
          width: "max-content",
          paddingInline: expanded
            ? `calc(${frameInset} - var(--focus-inset-edge))`
            : frameInset,
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
                setView(PREVIEW);
              } else {
                expansion.current = {
                  origin: event.currentTarget,
                  source: text,
                };
                setView(READING);
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
            onClick={() => setView(openFind)}
          />
        )}
      </CodeCaption>
      {view.mode === "finding" && (
        <div data-slot="output-controls" style={findBar}>
          <FindField
            t={t}
            label={t("output.search")}
            fieldLabel={t("output.field")}
            value={view.query}
            fieldRef={search}
            status={
              view.query && (
                <span role="status">
                  {matches.length
                    ? t("output.matchCount", {
                        current: (view.match % matches.length) + 1,
                        total: matches.length,
                      })
                    : t("output.noMatches")}
                </span>
              )
            }
            onChange={(query) => setView({ mode: "finding", query, match: 0 })}
            onClose={closeFind}
            onEnter={(backward) => {
              if (matches.length)
                setView((current) =>
                  current.mode === "finding"
                    ? {
                        ...current,
                        match:
                          (current.match +
                            (backward ? matches.length - 1 : 1)) %
                          matches.length,
                      }
                    : current,
                );
            }}
          />
        </div>
      )}
      <div
        role={expanded ? undefined : "region"}
        aria-label={expanded ? undefined : t("output.preview")}
        tabIndex={expanded ? undefined : 0}
        {...(expanded ? undefined : readerSourceProps)}
        {...TOOL_OUTPUT_QUOTE_SOURCE}
        onKeyDown={(event) => {
          selectAllInReader(event);
          if (!expanded && event.cancelable && !isImeKey(event.nativeEvent))
            scrollWithHorizontalArrow(event);
          if (expanded && isFindShortcut(event)) {
            event.preventDefault();
            event.stopPropagation();
            setView(openFind);
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
          overflowX: expanded ? undefined : "auto",
          overscrollBehaviorX: expanded ? undefined : "contain",
          paddingBlockEnd: expanded ? undefined : frameInset,
        }}
      >
        {expanded ? (
          <ReadingList
            ref={reader}
            className="reading-focus-edge"
            data={lineIndices}
            keyExtractor={String}
            label={t("output.region")}
            maintainScrollAtEndThreshold={0}
            onFollowChange={(list, value) => {
              setAtEnd(value);
              const node = list.getScrollableNode();
              if (node) markReaderSource(node);
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

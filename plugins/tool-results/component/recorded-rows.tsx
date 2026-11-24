import {
  Fragment,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type Ref,
} from "react";
import { Search } from "@flame/icons/react";
import type { Translate } from "@flame/component-sdk";
import {
  Button,
  CodeCaption,
  CodeFrame,
  IconButton,
  Metadata,
  RecordBody,
  RecordMeta,
  copyReaderSelection,
  RecordSummary,
  selectAllInReader,
} from "@flame/ui";
import { renderAnsi } from "./ansi";
import {
  containsQuery,
  enterFind,
  findBar,
  FindField,
  isFindShortcut,
} from "./find";
import { frameInset, resultActions } from "./frame";
import { rowText, type Row } from "./result-content";

const RESULT_PAGE_ROWS = 40;

export function RecordedRows({
  label,
  source,
  recorded,
  continues,
  notice,
  emptyMessage,
  t,
}: {
  label: string;
  source: string;
  recorded: Row[];
  continues?: boolean;
  notice?: string;
  emptyMessage?: string;
  t: Translate;
}) {
  const [unfilteredVisible, setUnfilteredVisible] = useState(RESULT_PAGE_ROWS);
  const [find, setFind] = useState<{ query: string; visible: number } | null>(
    null,
  );
  const finding = find !== null;
  const filter = find?.query ?? "";
  const visible = find?.query ? find.visible : unfilteredVisible;
  const viewport = useRef<HTMLDivElement>(null);
  const unfilteredScrollTop = useRef<number | null>(null);
  const search = useRef<HTMLInputElement>(null);
  const firstRevealed = useRef<HTMLDivElement>(null);
  const continuation = useRef<{
    origin: HTMLButtonElement;
    activeElement: Element | null;
  } | null>(null);
  useLayoutEffect(() => {
    const intent = continuation.current;
    continuation.current = null;
    const node = viewport.current;
    const row = firstRevealed.current;
    if (
      !intent ||
      !node ||
      !row ||
      (node.ownerDocument.activeElement !== intent.origin &&
        node.ownerDocument.activeElement !== intent.activeElement &&
        !(
          intent.activeElement === intent.origin &&
          !intent.origin.isConnected &&
          node.ownerDocument.activeElement === node.ownerDocument.body
        ))
    )
      return;
    node.scrollTop +=
      row.getBoundingClientRect().top -
      node.getBoundingClientRect().top -
      node.clientTop;
    node.focus();
  }, [visible]);
  useLayoutEffect(() => {
    if (finding) enterFind(search.current);
  }, [finding]);
  useLayoutEffect(() => {
    const node = viewport.current;
    if (!node) return;
    if (filter) {
      node.scrollTop = 0;
    } else if (unfilteredScrollTop.current !== null) {
      node.scrollTop = unfilteredScrollTop.current;
      unfilteredScrollTop.current = null;
    }
  }, [filter]);
  function openFind() {
    setFind((current) => current ?? { query: "", visible: RESULT_PAGE_ROWS });
  }
  function changeFilter(query: string) {
    if (query === filter) return;
    // Empty Find still uses the normal reader and its current position.
    if (!filter)
      unfilteredScrollTop.current = viewport.current?.scrollTop ?? null;
    continuation.current = null;
    setFind({ query, visible: RESULT_PAGE_ROWS });
  }
  function closeFind() {
    if (filter) continuation.current = null;
    setFind(null);
    viewport.current?.focus({ preventScroll: true });
  }
  function showMore(origin: HTMLButtonElement) {
    continuation.current = {
      origin,
      activeElement: origin.ownerDocument.activeElement,
    };
    if (filter) {
      setFind((current) =>
        current?.query === filter
          ? { ...current, visible: current.visible + RESULT_PAGE_ROWS }
          : current,
      );
    } else {
      setUnfilteredVisible((current) => current + RESULT_PAGE_ROWS);
    }
  }
  const matching = useMemo(
    () =>
      filter
        ? recorded.filter((row) => containsQuery(rowText(row), filter))
        : recorded,
    [recorded, filter],
  );
  const presented = matching.slice(0, visible);
  const lastSource = recorded.findLast((row) => row.kind === "source");
  const numberWidth = `${String(lastSource?.number ?? 1).length}ch`;
  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        minWidth: 0,
        maxHeight: "min(50vh, 24rem)",
      }}
    >
      <CodeFrame data-slot="recorded-result">
        <CodeCaption
          language={label}
          code={source}
          labels={resultActions(t, t("result.copy"))}
          wrappable={false}
        >
          {continues && <RecordMeta>{t("result.morePages")}</RecordMeta>}
          {notice && <RecordMeta>{notice}</RecordMeta>}
          {recorded.length > RESULT_PAGE_ROWS && (
            <span role="status" style={{ fontVariantNumeric: "tabular-nums" }}>
              {filter
                ? t("result.countMatches", {
                    shown: Math.min(visible, matching.length),
                    total: matching.length,
                  })
                : t("result.countRows", {
                    shown: Math.min(visible, matching.length),
                    total: matching.length,
                  })}
            </span>
          )}
          {!finding && (
            <IconButton
              icon={Search}
              size="xs"
              quiet
              aria-label={t("find.open")}
              onClick={openFind}
            />
          )}
        </CodeCaption>
        {finding && (
          <div data-slot="recorded-result-controls" style={findBar}>
            <FindField
              t={t}
              label={t("result.search")}
              fieldLabel={t("result.filter")}
              value={filter}
              fieldRef={search}
              onChange={changeFilter}
              onClose={closeFind}
            />
          </div>
        )}
        <div
          ref={viewport}
          data-slot="recorded-result-viewport"
          role="region"
          aria-label={t("result.region")}
          tabIndex={0}
          data-focus-inset=""
          style={{
            minWidth: 0,
            minHeight: 0,
            overflow: "auto",
            overscrollBehavior: "contain auto",
            scrollPaddingBlock: "var(--layout-control-gap)",
            paddingInline: frameInset,
            paddingBlockEnd: frameInset,
          }}
          onKeyDown={(event) => {
            if (isFindShortcut(event)) {
              event.preventDefault();
              event.stopPropagation();
              openFind();
              enterFind(search.current);
            }
            selectAllInReader(event);
          }}
          onCopy={(event) =>
            copyReaderSelection(event, presented.map(rowText).join("\n"))
          }
        >
          {/* Select All bounds its Range by this region's child offsets, which
              filtering and paging would shift; one stable child keeps it whole. */}
          <div>
            {presented.map((row, index) => (
              <RecordedRow
                key={index}
                ref={
                  index === visible - RESULT_PAGE_ROWS
                    ? firstRevealed
                    : undefined
                }
                row={row}
                first={index === 0}
                numberWidth={numberWidth}
                t={t}
              />
            ))}
            {!recorded.length ? (
              <Metadata>{emptyMessage ?? t("result.empty")}</Metadata>
            ) : (
              !matching.length && <Metadata>{t("result.noMatches")}</Metadata>
            )}
            {matching.length > visible && (
              <Button
                onClick={(event) => showMore(event.currentTarget)}
                style={{ marginBlockStart: "var(--layout-control-gap)" }}
              >
                {t("result.showMore", {
                  rows: Math.min(RESULT_PAGE_ROWS, matching.length - visible),
                })}
              </Button>
            )}
          </div>
        </div>
      </CodeFrame>
    </div>
  );
}

// Code-like rows keep the monospaced reading face; entries and prose do not.
const MONO_ROWS: ReadonlySet<Row["kind"]> = new Set(["source", "located"]);

function rowStyle(
  row: Row,
  first: boolean,
  numberWidth: string,
): CSSProperties {
  const mono = MONO_ROWS.has(row.kind);
  const grouped = row.kind === "located" || row.kind === "entry";
  return {
    display: "grid",
    gridTemplateColumns:
      row.kind === "source"
        ? `${numberWidth} minmax(0, 1fr)`
        : "minmax(0, 1fr)",
    columnGap: "var(--layout-content-gap)",
    rowGap: "var(--layout-label-gap)",
    paddingBlockStart:
      first || !grouped
        ? undefined
        : row.body === undefined
          ? "var(--layout-label-gap)"
          : "calc(var(--layout-row-padding) * 2)",
    minWidth: 0,
    fontFamily: mono ? "var(--font-mono)" : "var(--font-sans)",
    fontSize: mono ? "var(--fs-code)" : "var(--fs-ui-md)",
    lineHeight: "var(--leading-relaxed)",
  };
}

function RecordedRow({
  ref,
  row,
  first,
  numberWidth,
  t,
}: {
  ref?: Ref<HTMLDivElement>;
  row: Row;
  first: boolean;
  numberWidth: string;
  t: Translate;
}) {
  return (
    <div ref={ref} style={rowStyle(row, first, numberWidth)}>
      {row.kind === "source" && (
        <span
          style={{
            color: "var(--color-text-faint)",
            textAlign: "end",
            userSelect: "none",
            fontVariantNumeric: "tabular-nums",
          }}
        >
          <span className="sr-only">{t("result.line")}</span>
          {row.number}
        </span>
      )}
      {row.kind === "located" && row.location !== undefined && (
        <Metadata
          style={{
            whiteSpace: "pre-wrap",
          }}
        >
          {row.location.split("/").map((part, index) => (
            <Fragment key={index}>
              {index > 0 && "/"}
              {index > 0 && <wbr />}
              {part}
            </Fragment>
          ))}
        </Metadata>
      )}
      {row.kind === "entry" && (
        <RecordSummary
          title={<span style={{ whiteSpace: "pre-wrap" }}>{row.title}</span>}
          description={
            row.context === undefined ? undefined : (
              <span style={{ whiteSpace: "pre-wrap" }}>{row.context}</span>
            )
          }
        />
      )}
      {row.body !== undefined &&
        (row.kind === "entry" || row.kind === "prose") && (
          <RecordBody>{row.body || " "}</RecordBody>
        )}
      {row.kind === "notice" && <Metadata>{row.body}</Metadata>}
      {row.body !== undefined &&
        (row.kind === "source" || row.kind === "located") && (
          <pre
            style={{
              whiteSpace: "pre-wrap",
              overflowWrap: "anywhere",
              margin: 0,
              color: /^error\b/.test(row.body)
                ? "var(--color-negative)"
                : /^warning\b/.test(row.body)
                  ? "var(--color-warning)"
                  : undefined,
            }}
          >
            {renderAnsi(row.body || " ")}
          </pre>
        )}
    </div>
  );
}

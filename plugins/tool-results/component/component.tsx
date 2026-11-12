import { ansiText, renderAnsi } from "./ansi";
import {
  enterFind,
  findBar,
  FindField,
  frameInset,
  isFindShortcut,
  RecordedOutput,
  resultActions,
} from "./recorded-output";
import { Fragment, useLayoutEffect, useMemo, useRef, useState } from "react";
import { Search } from "@flame/icons/react";
import {
  type CapturedToolResult,
  type ComponentContext,
  type Translate,
  useTranslator,
} from "@flame/component-sdk";
import {
  Button,
  CodeCaption,
  CodeFrame,
  IconButton,
  Metadata,
  RecordBody,
  RecordMeta,
  copyWholeReader,
  RecordSummary,
  selectAllInReader,
} from "@flame/ui";
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { de } from "./locales/de";

const CATALOGS = { en, zh, "zh-TW": zhTW, ja, ko, es, fr, de };

type Row =
  | { kind: "source"; number: number; body: string }
  | { kind: "located"; location?: string; body?: string }
  | { kind: "entry"; title: string; context?: string; body?: string }
  | { kind: "prose"; body: string }
  | { kind: "notice"; body: string };

type ResultContent =
  | {
      kind: "rows";
      label: string;
      rows: Row[];
      continues?: boolean;
      notice?: string;
      emptyMessage?: string;
    }
  | { kind: "output"; text: string }
  | { kind: "changes"; changes: Record<string, unknown>[] };

const RESULT_PAGE_ROWS = 40;

function record(value: unknown): Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}
function parsed(value: string): Record<string, unknown> {
  try {
    return record(JSON.parse(value));
  } catch {
    return {};
  }
}
function text(value: unknown): string {
  return String(value ?? "");
}

function sourceRows(source: string, start = 1): Row[] {
  return source.split("\n").map((body, index) => ({
    kind: "source",
    number: start + index,
    body,
  }));
}

function proseRows(body: string): Row[] {
  return [{ kind: "prose", body }];
}

interface SkillPage {
  content?: string;
  skills?: { name: string; description: string }[];
  next_cursor?: { offset: number; snapshot: string };
}

function skillPage(
  source: string,
  member: "content" | "skills",
): SkillPage | undefined {
  const page = parsed(source);
  if (
    !Object.hasOwn(page, member) ||
    Object.keys(page).some((key) => key !== member && key !== "next_cursor")
  )
    return undefined;
  if (member === "content" && typeof page.content !== "string")
    return undefined;
  if (
    member === "skills" &&
    (!Array.isArray(page.skills) ||
      !page.skills.every((skill) => {
        const value = record(skill);
        return (
          Object.keys(value).length === 2 &&
          typeof value.name === "string" &&
          typeof value.description === "string"
        );
      }))
  )
    return undefined;
  if (page.next_cursor !== undefined) {
    const cursor = record(page.next_cursor);
    if (
      Object.keys(cursor).length !== 2 ||
      typeof cursor.offset !== "number" ||
      !Number.isSafeInteger(cursor.offset) ||
      cursor.offset <= 0 ||
      typeof cursor.snapshot !== "string" ||
      !/^[0-9a-f]{64}$/i.test(cursor.snapshot)
    )
      return undefined;
  }
  return page as SkillPage;
}

function skillPageRows(page: SkillPage, rows: Row[]): Row[] {
  return page.next_cursor
    ? [
        ...rows,
        {
          kind: "located",
          location: "next_cursor",
          body: JSON.stringify(page.next_cursor, null, 2),
        },
      ]
    : rows;
}

function webPage(page: Record<string, unknown>) {
  const { snapshotId, url, format, content, offset, totalRunes, nextOffset } =
    page;
  if (
    Object.keys(page).some(
      (key) =>
        ![
          "snapshotId",
          "url",
          "format",
          "content",
          "offset",
          "totalRunes",
          "nextOffset",
        ].includes(key),
    ) ||
    typeof snapshotId !== "string" ||
    !/^[0-9a-f]{64}$/.test(snapshotId) ||
    typeof url !== "string" ||
    !url ||
    typeof format !== "string" ||
    !["markdown", "html", "text"].includes(format) ||
    typeof content !== "string" ||
    typeof offset !== "number" ||
    !Number.isSafeInteger(offset) ||
    offset < 0 ||
    typeof totalRunes !== "number" ||
    !Number.isSafeInteger(totalRunes) ||
    totalRunes < offset
  )
    return undefined;
  const end = offset + Array.from(content).length;
  if (
    end > totalRunes ||
    (nextOffset === undefined
      ? end !== totalRunes
      : typeof nextOffset !== "number" ||
        !Number.isSafeInteger(nextOffset) ||
        nextOffset !== end ||
        nextOffset <= offset ||
        nextOffset >= totalRunes)
  )
    return undefined;
  return {
    snapshotId,
    url,
    format,
    content,
    offset,
    totalRunes,
    ...(typeof nextOffset === "number" ? { nextOffset } : {}),
  };
}

function httpContent(value: unknown): string | undefined {
  const content = record(value);
  if (Object.keys(content).length !== 2 || typeof content.data !== "string")
    return undefined;
  if (content.encoding === "utf8") return content.data;
  if (content.encoding === "base64") return `base64: ${content.data}`;
  return undefined;
}

function httpResponse(response: Record<string, unknown>) {
  const { status, duration, truncated } = response;
  if (
    Object.keys(response).some(
      (key) =>
        !["status", "duration", "truncated", "headers", "body"].includes(key),
    ) ||
    typeof status !== "number" ||
    !Number.isInteger(status) ||
    typeof duration !== "string" ||
    (truncated !== undefined && typeof truncated !== "boolean")
  )
    return undefined;
  const body = httpContent(response.body);
  if (body === undefined) return undefined;
  const headers: string[] = [];
  if (response.headers !== undefined) {
    if (
      !response.headers ||
      typeof response.headers !== "object" ||
      Array.isArray(response.headers)
    )
      return undefined;
    for (const [name, values] of Object.entries(response.headers)) {
      if (!Array.isArray(values) || values.length === 0) return undefined;
      for (const value of values) {
        const content = httpContent(value);
        if (content === undefined) return undefined;
        headers.push(`${name}: ${content}`);
      }
    }
  }
  return { status, duration, truncated, headers: headers.join("\n"), body };
}

function resultContent(
  renderer: string,
  tool: CapturedToolResult,
  t: Translate,
): ResultContent {
  const source = tool.result ?? "";
  switch (renderer) {
    case "shell":
    case "read-shell-output":
      return { kind: "output", text: source };
    case "read":
      return {
        kind: "rows",
        label: t("caption.source"),
        rows: sourceRows(source.replace(/\n$/, ""), tool.range?.start ?? 1),
      };
    case "ask-user":
      return {
        kind: "rows",
        label: t("caption.answer"),
        rows: proseRows(source),
      };
    case "load-skill":
    case "read-skill-resource": {
      const page = skillPage(source, "content");
      const body = page?.content ?? source;
      const rows =
        renderer === "load-skill" ? proseRows(body) : sourceRows(body);
      return {
        kind: "rows",
        label: t(
          renderer === "load-skill" ? "caption.instructions" : "caption.result",
        ),
        rows: page ? skillPageRows(page, rows) : rows,
        continues: page?.next_cursor !== undefined,
      };
    }
    case "search-memory":
      return {
        kind: "rows",
        label: t("caption.memories"),
        rows: proseRows(source),
      };
    case "list-skills": {
      const page = skillPage(source, "skills");
      return {
        kind: "rows",
        label: t("caption.skills"),
        rows: page
          ? skillPageRows(
              page,
              page.skills!.map((skill) => ({
                kind: "entry",
                title: skill.name,
                body: skill.description,
              })),
            )
          : sourceRows(source),
        continues: page?.next_cursor !== undefined,
        emptyMessage: t("skills.empty"),
      };
    }
    case "read-tool-result":
    case "search-tools":
    case "lsp":
    case "stop-shell":
    case "propose-skill":
    case "delegate-task":
      return {
        kind: "rows",
        label: t("caption.result"),
        rows: sourceRows(source),
      };
  }
  const result = parsed(source);
  if (renderer === "http-request") {
    const response = httpResponse(result);
    return response
      ? {
          kind: "rows",
          label: t("caption.response"),
          rows: [
            {
              kind: "located",
              location: `${response.status} ${response.duration}${response.truncated ? ` · ${t("http.truncated")}` : ""}`,
              body: `${response.headers}\n${response.body}`,
            },
          ],
        }
      : { kind: "rows", label: t("caption.result"), rows: sourceRows(source) };
  }
  if (renderer === "web-fetch" || renderer === "web-browse") {
    const page = webPage(result);
    return page
      ? {
          kind: "rows",
          label: page.format,
          continues: page.nextOffset !== undefined,
          rows: [
            { kind: "located", location: page.url, body: page.content },
            {
              kind: "located",
              location: "snapshotId / offset / totalRunes / nextOffset",
              body: JSON.stringify(
                {
                  snapshotId: page.snapshotId,
                  offset: page.offset,
                  totalRunes: page.totalRunes,
                  ...(page.nextOffset === undefined
                    ? {}
                    : { nextOffset: page.nextOffset }),
                },
                null,
                2,
              ),
            },
          ],
        }
      : { kind: "rows", label: t("caption.result"), rows: sourceRows(source) };
  }
  if (renderer === "web-search" && Array.isArray(result.results))
    return {
      kind: "rows",
      label: t("caption.results"),
      rows: result.results.map((hit) => {
        const value = record(hit);
        const title = text(value.title).trim();
        const url = text(value.url);
        const context = [
          title && title !== url ? url : undefined,
          typeof value.source === "string" ? value.source : undefined,
          typeof value.published_time === "string"
            ? value.published_time
            : undefined,
        ]
          .filter((part) => part !== undefined)
          .join("\n");
        return {
          kind: "entry",
          title: title || url,
          context: context || undefined,
          body: value.snippet === undefined ? undefined : text(value.snippet),
        };
      }),
    };
  // A shape-selected result renders by the shape its tool declared; member
  // names alone never decide what a result means.
  switch (tool.resultShape) {
    case "patch":
      if (Array.isArray(result.changes))
        return { kind: "changes", changes: result.changes.map(record) };
      break;
    case "search": {
      const content = searchContent(result, t);
      if (content) return content;
      break;
    }
    case "command":
      if (typeof result.output === "string")
        return {
          kind: "rows",
          label: t("caption.result"),
          rows: sourceRows(result.output),
        };
      break;
  }
  return { kind: "rows", label: t("caption.result"), rows: sourceRows(source) };
}

function searchContent(
  result: Record<string, unknown>,
  t: Translate,
): ResultContent | undefined {
  if (!Array.isArray(result.hits)) return undefined;
  return {
    kind: "rows",
    label: t("caption.matches"),
    notice:
      result.complete === false
        ? t("search.partial", {
            total: text(result.total),
            unread: text(record(result.coverage).remainingFiles),
            skipped: text(record(result.coverage).skippedFiles),
          })
        : result.truncated === true
          ? t("search.truncated", {
              shown: result.hits.length,
              total: text(result.total),
            })
          : undefined,
    rows: result.hits.map((hit) => {
      const value = record(hit);
      return {
        kind: "located",
        location: `${text(value.path)}${value.lineNumber === undefined ? "" : `:${text(value.lineNumber)}`}`,
        body: value.snippet === undefined ? undefined : text(value.snippet),
      };
    }),
  };
}

function rowText(row: Row): string {
  switch (row.kind) {
    case "source":
      return ansiText(row.body);
    case "located":
      return [row.location, row.body && ansiText(row.body)]
        .filter((part) => part !== undefined)
        .join("\n");
    case "entry":
      return [row.title, row.context, row.body]
        .filter((part) => part !== undefined)
        .join("\n");
    case "prose":
    case "notice":
      return row.body;
  }
}

function RecordedRows({
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
  const [finding, setFinding] = useState(false);
  const [filter, setFilter] = useState("");
  const [visible, setVisible] = useState(RESULT_PAGE_ROWS);
  const viewport = useRef<HTMLDivElement>(null);
  const search = useRef<HTMLElement>(null);
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
    if (viewport.current) viewport.current.scrollTop = 0;
  }, [filter]);
  function closeFind() {
    setFinding(false);
    setFilter("");
    setVisible(RESULT_PAGE_ROWS);
    viewport.current?.focus();
  }
  const matching = useMemo(
    () =>
      recorded.filter((row) => {
        const identity =
          row.kind === "entry"
            ? `${row.title}\n${row.context ?? ""}`
            : row.kind === "located"
              ? (row.location ?? "")
              : "";
        const body = row.body ?? "";
        const visibleBody =
          row.kind === "source" || row.kind === "located"
            ? ansiText(body)
            : body;
        return `${identity}\n${visibleBody}`
          .toLowerCase()
          .includes(filter.toLowerCase());
      }),
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
              onClick={() => setFinding(true)}
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
              onChange={(value) => {
                setFilter(value);
                setVisible(RESULT_PAGE_ROWS);
              }}
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
              setFinding(true);
              enterFind(search.current);
            }
            selectAllInReader(event);
          }}
          onCopy={(event) =>
            copyWholeReader(event, presented.map(rowText).join("\n"))
          }
        >
          {/* Select All bounds its Range by this region's child offsets, which
              filtering and paging would shift; one stable child keeps it whole. */}
          <div>
            {presented.map((row, index) => (
              <div
                key={index}
                ref={
                  index === visible - RESULT_PAGE_ROWS
                    ? firstRevealed
                    : undefined
                }
                style={{
                  display: "grid",
                  gridTemplateColumns:
                    row.kind === "source"
                      ? `${numberWidth} minmax(0, 1fr)`
                      : "minmax(0, 1fr)",
                  columnGap: "var(--layout-content-gap)",
                  rowGap: "var(--layout-label-gap)",
                  paddingBlockStart:
                    index === 0 ||
                    (row.kind !== "located" && row.kind !== "entry")
                      ? undefined
                      : row.body === undefined
                        ? "var(--layout-label-gap)"
                        : "calc(var(--layout-row-padding) * 2)",
                  minWidth: 0,
                  fontFamily:
                    row.kind === "source" || row.kind === "located"
                      ? "var(--font-mono)"
                      : "var(--font-sans)",
                  fontSize:
                    row.kind === "source" || row.kind === "located"
                      ? "var(--fs-code)"
                      : "var(--fs-ui-md)",
                  lineHeight: "var(--leading-relaxed)",
                }}
              >
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
                    title={
                      <span style={{ whiteSpace: "pre-wrap" }}>
                        {row.title}
                      </span>
                    }
                    description={
                      row.context === undefined ? undefined : (
                        <span style={{ whiteSpace: "pre-wrap" }}>
                          {row.context}
                        </span>
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
            ))}
            {!recorded.length ? (
              <Metadata>{emptyMessage ?? t("result.empty")}</Metadata>
            ) : (
              !matching.length && <Metadata>{t("result.noMatches")}</Metadata>
            )}
            {matching.length > visible && (
              <Button
                onClick={(event) => {
                  continuation.current = {
                    origin: event.currentTarget,
                    activeElement:
                      event.currentTarget.ownerDocument.activeElement,
                  };
                  setVisible((current) => current + RESULT_PAGE_ROWS);
                }}
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

const CHANGE_VERB = {
  added: "change.added",
  modified: "change.modified",
  moved: "change.moved",
  deleted: "change.deleted",
} as const;

export function RecordedResult({ context }: { context: ComponentContext }) {
  const t = useTranslator(CATALOGS, context.locale);
  const tool = context.toolResult;
  const content = useMemo(() => {
    if (!tool || tool.status === "running" || typeof tool.result !== "string")
      throw new Error("A settled captured Tool result is required");
    return resultContent(context.component.componentId, tool, t);
  }, [context.component.componentId, tool, t]);
  if (content.kind === "changes")
    return !content.changes.length ? (
      <Metadata>{t("changes.empty")}</Metadata>
    ) : (
      <table
        aria-label={t("changes.applied")}
        style={{ width: "100%", tableLayout: "fixed" }}
      >
        <tbody>
          {content.changes.map((change, index) => {
            const path = text(change.path);
            const status = text(change.status);
            const verb = CHANGE_VERB[status as keyof typeof CHANGE_VERB];
            return (
              <tr key={`${path}:${index}`}>
                <td
                  style={{
                    width: "5em",
                    verticalAlign: "baseline",
                    color: "var(--color-text-muted)",
                    fontSize: "var(--fs-ui-sm)",
                  }}
                >
                  {verb ? t(verb) : status}
                </td>
                <td
                  style={{
                    verticalAlign: "baseline",
                    overflowWrap: "anywhere",
                    fontFamily: "var(--font-mono)",
                    fontSize: "var(--fs-code)",
                  }}
                >
                  <span title={path}>{path}</span>
                  {typeof change.from === "string" && (
                    <Metadata>
                      {t("changes.from", { from: change.from })}
                    </Metadata>
                  )}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    );
  if (content.kind === "output")
    return (
      <>
        {tool?.outputPage && (
          <RecordMeta>
            {t("output.byteRange", {
              start: tool.outputPage.offset,
              end: tool.outputPage.nextOffset,
            })}
            {tool.outputPage.shellId &&
              ` · ${t("output.shell", { id: tool.outputPage.shellId })}`}
            {tool.outputPage.status && ` · ${tool.outputPage.status}`}
            {tool.outputPage.hasMore && ` · ${t("result.morePages")}`}
            {tool.outputPage.dropped && ` · ${t("output.incomplete")}`}
          </RecordMeta>
        )}
        {content.text === "" ? (
          <Metadata>{t("output.empty")}</Metadata>
        ) : (
          <RecordedOutput text={content.text} t={t} />
        )}
      </>
    );
  return (
    <RecordedRows
      label={content.label}
      source={tool!.result!}
      recorded={content.rows}
      continues={content.continues}
      notice={content.notice}
      emptyMessage={content.emptyMessage}
      t={t}
    />
  );
}

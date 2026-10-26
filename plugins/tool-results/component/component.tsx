import { ansiText, renderAnsi } from "./ansi";
import {
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
  createTranslator,
  type CapturedToolResult,
  type ComponentContext,
  type Translate,
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

function xmlElements(element: Element): Element[] | undefined {
  if (
    element.attributes.length ||
    Array.from(element.childNodes).some(
      (node) =>
        node.nodeType !== Node.ELEMENT_NODE &&
        !(node.nodeType === Node.TEXT_NODE && !node.textContent?.trim()),
    )
  )
    return undefined;
  return Array.from(element.children);
}

function skillRows(source: string, t: Translate): Row[] {
  const document = new DOMParser().parseFromString(source, "application/xml");
  const root = document.documentElement;
  if (!root) return sourceRows(source);
  const entries = xmlElements(root);
  if (
    document.childNodes.length !== 1 ||
    root.tagName !== "available_skills" ||
    document.querySelector("parsererror") ||
    !entries
  )
    return sourceRows(source);
  const rows: Row[] = [];
  for (const [index, element] of entries.entries()) {
    if (
      element.tagName === "truncated" &&
      !element.attributes.length &&
      element.childNodes.length === 1 &&
      element.firstChild?.nodeType === Node.TEXT_NODE &&
      element.textContent === "true" &&
      index === entries.length - 1
    ) {
      rows.push({
        kind: "notice",
        body: t("skills.truncated"),
      });
      continue;
    }
    const fields = xmlElements(element);
    if (
      element.tagName !== "skill" ||
      !fields ||
      fields.length !== 2 ||
      fields[0]!.tagName !== "name" ||
      fields[1]!.tagName !== "description" ||
      fields.some(
        (field) =>
          field.attributes.length ||
          Array.from(field.childNodes).some(
            (node) => node.nodeType !== Node.TEXT_NODE,
          ),
      )
    )
      return sourceRows(source);
    rows.push({
      kind: "entry",
      title: fields[0]!.textContent ?? "",
      body: fields[1]!.textContent ?? "",
    });
  }
  return rows.length ? rows : [{ kind: "notice", body: t("skills.empty") }];
}

function resultContent(tool: CapturedToolResult, t: Translate): ResultContent {
  const source = tool.result ?? "";
  switch (tool.name) {
    case "shell":
    case "read_shell_output":
      return { kind: "output", text: source };
    case "read":
      return {
        kind: "rows",
        label: t("caption.source"),
        rows: sourceRows(source.replace(/\n$/, ""), tool.range?.start ?? 1),
      };
    case "ask_user":
      return {
        kind: "rows",
        label: t("caption.answer"),
        rows: proseRows(source),
      };
    case "load_skill":
    case "read_skill_resource": {
      const page = skillPage(source, "content");
      const body = page?.content ?? source;
      const rows =
        tool.name === "load_skill" ? proseRows(body) : sourceRows(body);
      return {
        kind: "rows",
        label: t(
          tool.name === "load_skill"
            ? "caption.instructions"
            : "caption.result",
        ),
        rows: page ? skillPageRows(page, rows) : rows,
        continues: page?.next_cursor !== undefined,
      };
    }
    case "search_memory":
      return {
        kind: "rows",
        label: t("caption.memories"),
        rows: proseRows(source),
      };
    case "list_skills": {
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
          : skillRows(source, t),
        continues: page?.next_cursor !== undefined,
        emptyMessage: t("skills.empty"),
      };
    }
    case "read_tool_result":
    case "search_tools":
    case "lsp":
    case "stop_shell":
    case "propose_skill":
    case "delegate_task":
      return {
        kind: "rows",
        label: t("caption.result"),
        rows: sourceRows(source),
      };
  }
  const result = parsed(source);
  if (Array.isArray(result.changes))
    return { kind: "changes", changes: result.changes.map(record) };
  if (Array.isArray(result.hits))
    return {
      kind: "rows",
      label: t("caption.matches"),
      rows: result.hits.map((hit) => {
        const value = record(hit);
        return {
          kind: "located",
          location: `${text(value.path)}${value.lineNumber === undefined ? "" : `:${text(value.lineNumber)}`}`,
          body: value.snippet === undefined ? undefined : text(value.snippet),
        };
      }),
    };
  if (Array.isArray(result.results))
    return {
      kind: "rows",
      label: t("caption.results"),
      rows: result.results.map((hit) => {
        const value = record(hit);
        const title = text(value.title).trim();
        const url = text(value.url);
        return {
          kind: "entry",
          title: title || url,
          context: title && title !== url ? url : undefined,
          body: value.snippet === undefined ? undefined : text(value.snippet),
        };
      }),
    };
  if (tool.name === "http_request" && typeof result.status === "number") {
    const headers = Object.entries(record(result.headers))
      .map(([key, value]) => `${key}: ${text(value)}`)
      .join("\n");
    return {
      kind: "rows",
      label: t("caption.response"),
      rows: [
        {
          kind: "located",
          location: `${result.status} ${text(result.duration)}${result.truncated ? ` · ${t("http.truncated")}` : ""}`,
          body: `${headers}\n${text(result.body)}`,
        },
      ],
    };
  }
  if (tool.name === "web_fetch" && typeof result.content === "string")
    return {
      kind: "rows",
      label: text(result.format ?? "text"),
      rows: [{ kind: "located", body: result.content }],
    };
  return {
    kind: "rows",
    label: t("caption.result"),
    rows: sourceRows(
      typeof result.output === "string" ? result.output : source,
    ),
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
  emptyMessage,
  t,
}: {
  label: string;
  source: string;
  recorded: Row[];
  continues?: boolean;
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
    if (finding) search.current?.focus();
  }, [finding]);
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
              setFinding(true);
              search.current?.focus();
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
  const t = useMemo(
    () => createTranslator(CATALOGS, context.locale),
    [context.locale],
  );
  const tool = context.toolResult;
  const content = useMemo(() => {
    if (!tool || tool.status === "running" || typeof tool.result !== "string")
      throw new Error("A settled captured Tool result is required");
    return resultContent(tool, t);
  }, [tool, t]);
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
    return <RecordedOutput text={content.text} t={t} />;
  return (
    <RecordedRows
      label={content.label}
      source={tool!.result!}
      recorded={content.rows}
      continues={content.continues}
      emptyMessage={content.emptyMessage}
      t={t}
    />
  );
}

import type { CapturedToolResult, Translate } from "@flame/component-sdk";
import { ansiText } from "./ansi";

export type Row =
  | { kind: "source"; number: number; body: string }
  | { kind: "located"; location?: string; body?: string }
  | { kind: "entry"; title: string; context?: string; body?: string }
  | { kind: "prose"; body: string }
  | { kind: "notice"; body: string };

export type ResultContent =
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
export function text(value: unknown): string {
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

function httpResponseRow(
  response: Record<string, unknown>,
  t: Translate,
): Row | undefined {
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
  return {
    kind: "located",
    location: `${status} ${duration}${truncated ? ` · ${t("http.truncated")}` : ""}`,
    body: headers.length ? `${headers.join("\n")}\n\n${body}` : body,
  };
}

type Renderer = (
  source: string,
  tool: CapturedToolResult,
  t: Translate,
) => ResultContent;

function resultRows(source: string, t: Translate): ResultContent {
  return { kind: "rows", label: t("caption.result"), rows: sourceRows(source) };
}

const outputRenderer: Renderer = (source) => ({ kind: "output", text: source });

const sourceRenderer: Renderer = (source, _tool, t) => resultRows(source, t);

function proseRenderer(label: string): Renderer {
  return (source, _tool, t) => ({
    kind: "rows",
    label: t(label),
    rows: proseRows(source),
  });
}

function skillDocumentRenderer(
  label: string,
  rowsOf: (body: string) => Row[],
): Renderer {
  return (source, _tool, t) => {
    const page = skillPage(source, "content");
    const rows = rowsOf(page?.content ?? source);
    return {
      kind: "rows",
      label: t(label),
      rows: page ? skillPageRows(page, rows) : rows,
      continues: page?.next_cursor !== undefined,
    };
  };
}

const listSkills: Renderer = (source, _tool, t) => {
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
};

const httpRequest: Renderer = (source, _tool, t) => {
  const response = httpResponseRow(parsed(source), t);
  return response
    ? { kind: "rows", label: t("caption.response"), rows: [response] }
    : resultRows(source, t);
};

const webDocument: Renderer = (source, _tool, t) => {
  const page = webPage(parsed(source));
  if (!page) return resultRows(source, t);
  const position = {
    snapshotId: page.snapshotId,
    offset: page.offset,
    totalRunes: page.totalRunes,
    ...(page.nextOffset === undefined ? {} : { nextOffset: page.nextOffset }),
  };
  return {
    kind: "rows",
    label: page.format,
    continues: page.nextOffset !== undefined,
    rows: [
      { kind: "located", location: page.url, body: page.content },
      {
        kind: "located",
        location: Object.keys(position).join(" / "),
        body: JSON.stringify(position, null, 2),
      },
    ],
  };
};

function searchHit(hit: unknown): Row {
  const value = record(hit);
  const title = text(value.title).trim();
  const url = text(value.url);
  const context = [
    title && title !== url ? url : undefined,
    typeof value.source === "string" ? value.source : undefined,
    typeof value.published_time === "string" ? value.published_time : undefined,
  ]
    .filter((part) => part !== undefined)
    .join("\n");
  return {
    kind: "entry",
    title: title || url,
    context: context || undefined,
    body: value.snippet === undefined ? undefined : text(value.snippet),
  };
}

const webSearch: Renderer = (source, tool, t) => {
  const result = parsed(source);
  return Array.isArray(result.results)
    ? {
        kind: "rows",
        label: t("caption.results"),
        rows: result.results.map(searchHit),
      }
    : declaredShape(source, tool, t);
};

// A shape-selected result renders by the shape its tool declared; member
// names alone never decide what a result means.
const declaredShape: Renderer = (source, tool, t) => {
  const result = parsed(source);
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
        return resultRows(result.output, t);
      break;
  }
  return resultRows(source, t);
};

// Keyed by the component ids the package declares; a declared id without an
// entry renders by its tool's result shape.
const RENDERERS: Readonly<Record<string, Renderer>> = {
  shell: outputRenderer,
  "read-shell-output": outputRenderer,
  read: (source, tool, t) => ({
    kind: "rows",
    label: t("caption.source"),
    rows: sourceRows(source.replace(/\n$/, ""), tool.range?.start ?? 1),
  }),
  "ask-user": proseRenderer("caption.answer"),
  "search-memory": proseRenderer("caption.memories"),
  "load-skill": skillDocumentRenderer("caption.instructions", proseRows),
  "read-skill-resource": skillDocumentRenderer("caption.result", (body) =>
    sourceRows(body),
  ),
  "list-skills": listSkills,
  "read-tool-result": sourceRenderer,
  "search-tools": sourceRenderer,
  lsp: sourceRenderer,
  "stop-shell": sourceRenderer,
  "propose-skill": sourceRenderer,
  "delegate-task": sourceRenderer,
  "http-request": httpRequest,
  "web-fetch": webDocument,
  "web-browse": webDocument,
  "web-search": webSearch,
  "run-script": (source, _tool, t) => scriptContent(parsed(source), source, t),
};

export function resultContent(
  renderer: string,
  tool: CapturedToolResult,
  t: Translate,
): ResultContent {
  const render = Object.hasOwn(RENDERERS, renderer)
    ? RENDERERS[renderer]!
    : declaredShape;
  return render(tool.result ?? "", tool, t);
}

const SCRIPT_CALL_STATUSES = new Set([
  "succeeded",
  "failed",
  "rejected",
  "unconfirmed",
  "notDispatched",
]);

// Each row is one Tool call the script really made, in request order; the
// status word stays in the title so find can filter by outcome.
function scriptContent(
  result: Record<string, unknown>,
  source: string,
  t: Translate,
): ResultContent {
  if (typeof result.output !== "string" || !Array.isArray(result.calls))
    return {
      kind: "rows",
      label: t("caption.result"),
      rows: sourceRows(source),
    };
  const status = text(result.status);
  const entries: unknown[] = result.calls;
  const calls: Row[] = entries.map((entry) => {
    const call = record(entry);
    const callStatus = text(call.status);
    const outcome = SCRIPT_CALL_STATUSES.has(callStatus)
      ? t(`script.call.${callStatus}`)
      : callStatus;
    const body =
      typeof call.error === "string"
        ? call.error
        : call.result !== undefined
          ? typeof call.result === "string"
            ? call.result
            : JSON.stringify(call.result, null, 2)
          : typeof call.omittedBytes === "number"
            ? t("script.call.omitted", { bytes: call.omittedBytes })
            : undefined;
    return {
      kind: "entry",
      title: `${text(call.tool)} · ${outcome}`,
      context:
        [
          typeof call.source === "string" ? call.source : undefined,
          call.arguments === undefined
            ? undefined
            : JSON.stringify(call.arguments),
        ]
          .filter((part) => part !== undefined)
          .join("\n") || undefined,
      body,
    };
  });
  const counts = [...SCRIPT_CALL_STATUSES]
    .map((callStatus) => {
      const count = entries.filter(
        (call) => record(call).status === callStatus,
      ).length;
      return count ? `${t(`script.call.${callStatus}`)} ${count}` : "";
    })
    .filter((part) => part !== "")
    .join(" · ");
  return {
    kind: "rows",
    label: t("caption.script"),
    notice:
      [
        status === "completed" ? "" : t(`script.status.${status}`),
        text(result.error),
        counts,
      ]
        .filter((part) => part !== "")
        .join("\n") || undefined,
    rows: [
      {
        kind: "located",
        location: t("caption.output"),
        body: result.output === "" ? t("output.empty") : result.output,
      },
      ...calls,
    ],
  };
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

export function rowText(row: Row): string {
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

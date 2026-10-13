import { useMemo, useState, type CSSProperties } from "react";
import type { CapturedToolResult, ComponentContext } from "@flame/plugin-sdk";
import {
  Button,
  ListRow,
  Metadata,
  Panel,
  PanelTitle,
  TextField,
  Toolbar,
} from "@flame/ui";

type Row = { location?: string; number?: number; body: string };
type AnsiStyle = {
  tone?: string;
  bold?: boolean;
  dim?: boolean;
  underline?: boolean;
};
const tones = [
  "muted",
  "negative",
  "success",
  "warning",
  "accent",
  "accent",
  "accent",
  "muted",
];

function sgr(style: AnsiStyle, params: string): AnsiStyle {
  const codes =
    params === ""
      ? [0]
      : params.split(";").map((code) => Number.parseInt(code, 10) || 0);
  let next = { ...style };
  for (let index = 0; index < codes.length; index++) {
    const code = codes[index];
    if (code === 0) next = {};
    else if (code === 1) next.bold = true;
    else if (code === 2) next.dim = true;
    else if (code === 4) next.underline = true;
    else if (code === 22) {
      next.bold = false;
      next.dim = false;
    } else if (code === 24) next.underline = false;
    else if (code === 39) next.tone = undefined;
    else if (code >= 30 && code <= 37) next.tone = tones[code - 30];
    else if (code >= 90 && code <= 97) next.tone = tones[code - 90];
    else if (code === 38 || code === 48)
      index += codes[index + 1] === 5 ? 2 : 4;
  }
  return next;
}

// ANSI determines local text style only; result bytes never become markup or URLs.
function ansi(input: string) {
  const spans: { text: string; style: AnsiStyle }[] = [];
  let style: AnsiStyle = {};
  let cursor = 0;
  // eslint-disable-next-line no-control-regex -- Recorded terminal output contains ANSI escape bytes.
  for (const match of input.matchAll(/\u001b\[([0-9;?]*)([A-Za-z])/g)) {
    if (match.index > cursor)
      spans.push({ text: input.slice(cursor, match.index), style });
    if (match[2] === "m") style = sgr(style, match[1]);
    cursor = match.index + match[0].length;
  }
  if (cursor < input.length) spans.push({ text: input.slice(cursor), style });
  return spans.map(({ text, style: value }, index) => {
    const style: CSSProperties = {
      color:
        value.tone === "muted"
          ? "var(--color-text-muted)"
          : value.tone
            ? `var(--tone-${value.tone})`
            : undefined,
      fontWeight: value.bold ? "bold" : undefined,
      opacity: value.dim ? 0.7 : undefined,
      textDecoration: value.underline ? "underline" : undefined,
    };
    return (
      <span key={index} style={style}>
        {text}
      </span>
    );
  });
}

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

function rows(tool: CapturedToolResult): Row[] {
  const source = tool.result ?? "";
  const result = parsed(source);
  if (tool.name === "read")
    return source
      .replace(/\n$/, "")
      .split("\n")
      .map((body, index) => ({
        number: (tool.range?.start ?? 1) + index,
        body,
      }));
  if (Array.isArray(result.changes))
    return result.changes.map((change) => {
      const value = record(change);
      return {
        location: value.from
          ? `${text(value.from)} → ${text(value.path)}`
          : text(value.path),
        body: text(value.status ?? "changed"),
      };
    });
  if (Array.isArray(result.hits))
    return result.hits.map((hit) => {
      const value = record(hit);
      return {
        location: `${text(value.path)}${value.lineNumber === undefined ? "" : `:${text(value.lineNumber)}`}`,
        body: text(value.snippet),
      };
    });
  if (Array.isArray(result.results))
    return result.results.map((hit) => {
      const value = record(hit);
      return {
        location: `${text(value.title || value.url)}\n${text(value.url)}`,
        body: text(value.snippet),
      };
    });
  if (tool.name === "http_request" && typeof result.status === "number") {
    const headers = Object.entries(record(result.headers))
      .map(([key, value]) => `${key}: ${text(value)}`)
      .join("\n");
    return [
      {
        location: `${result.status} ${text(result.duration)}${result.truncated ? " · Truncated" : ""}`,
        body: `${headers}\n${text(result.body)}`,
      },
    ];
  }
  if (tool.name === "web_fetch" && typeof result.content === "string")
    return [{ location: text(result.format ?? "text"), body: result.content }];
  if (tool.name === "list_skills") {
    const entries = [
      ...source.matchAll(
        /<skill>\s*<name>([\s\S]*?)<\/name>\s*<description>([\s\S]*?)<\/description>/g,
      ),
    ];
    if (entries.length)
      return entries.map((entry) => ({
        location: entry[1].trim(),
        body: entry[2].trim(),
      }));
  }
  if (tool.name === "ask_user") {
    const answer = result.answer ?? result.response;
    if (typeof answer === "string") return [{ body: answer }];
  }
  return (typeof result.output === "string" ? result.output : source)
    .split("\n")
    .map((body) => ({ body }));
}

export function RecordedResult({ context }: { context: ComponentContext }) {
  const tool = context.toolResult;
  if (!tool || tool.status === "running" || typeof tool.result !== "string")
    throw new Error("A settled captured Tool result is required");
  const [filter, setFilter] = useState("");
  const [visible, setVisible] = useState(40);
  const recorded = useMemo(() => rows(tool), [tool]);
  const matching = useMemo(
    () =>
      recorded.filter((row) =>
        `${row.location ?? ""}\n${row.body}`
          .toLowerCase()
          .includes(filter.toLowerCase()),
      ),
    [recorded, filter],
  );
  return (
    <div style={{ maxHeight: "min(50vh, 24rem)", overflowY: "auto" }}>
      <Panel label="Recorded Tool result">
        <PanelTitle>{tool.name}</PanelTitle>
        <Metadata>
          {tool.status} · Recorded result{tool.error ? ` · ${tool.error}` : ""}
        </Metadata>
        <Toolbar label="Recorded result filter">
          <TextField
            aria-label="Filter recorded result"
            type="search"
            value={filter}
            onChange={(event) => {
              setFilter(event.target.value);
              setVisible(40);
            }}
          />
        </Toolbar>
        {matching.slice(0, visible).map((row, index) => (
          <ListRow key={index}>
            {row.location !== undefined && <Metadata>{row.location}</Metadata>}
            {row.number !== undefined && <Metadata>Line {row.number}</Metadata>}
            <pre
              style={{
                whiteSpace: "pre-wrap",
                overflowWrap: "anywhere",
                margin: 0,
                color: /^error\b/.test(row.body)
                  ? "var(--tone-negative)"
                  : /^warning\b/.test(row.body)
                    ? "var(--tone-warning)"
                    : undefined,
              }}
            >
              {ansi(row.body || " ")}
            </pre>
          </ListRow>
        ))}
        {!matching.length && (
          <p>No matching recorded rows. Clear the filter to see all results.</p>
        )}
        {matching.length > visible && (
          <Button onClick={() => setVisible((current) => current + 40)}>
            Show {Math.min(40, matching.length - visible)} more matching rows
          </Button>
        )}
      </Panel>
    </div>
  );
}

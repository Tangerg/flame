import { ansiText, renderAnsi } from "./ansi";
import { RecordedOutput } from "./recorded-output";
import { useMemo, useState } from "react";
import type { CapturedToolResult, ComponentContext } from "@flame/plugin-sdk";
import {
  Button,
  ListRow,
  Metadata,
  Panel,
  PanelTitle,
  SearchField,
  Toolbar,
} from "@flame/ui";

type Row = { location?: string; number?: number; body: string };

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
        location: entry[1]!.trim(),
        body: entry[2]!.trim(),
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

function RecordedRows({ tool }: { tool: CapturedToolResult }) {
  const [filter, setFilter] = useState("");
  const [visible, setVisible] = useState(40);
  const recorded = useMemo(() => rows(tool), [tool]);
  const matching = useMemo(
    () =>
      recorded.filter((row) =>
        `${row.location ?? ""}\n${ansiText(row.body)}`
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
          <SearchField
            aria-label="Filter recorded result"
            placeholder="Find in recorded results"
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
              {renderAnsi(row.body || " ")}
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

const CHANGE_VERB = {
  added: "Created",
  modified: "Edited",
  moved: "Moved",
  deleted: "Deleted",
} as const;

export function RecordedResult({ context }: { context: ComponentContext }) {
  const tool = context.toolResult;
  if (!tool || tool.status === "running" || typeof tool.result !== "string")
    throw new Error("A settled captured Tool result is required");
  const changes = parsed(tool.result).changes;
  if (Array.isArray(changes))
    return (
      <table
        aria-label="Applied changes"
        style={{ width: "100%", tableLayout: "fixed" }}
      >
        <tbody>
          {changes.map((input, index) => {
            const change = record(input);
            const path = text(change.path);
            const status = text(change.status);
            return (
              <tr key={`${path}:${index}`}>
                <td style={{ width: "5em", verticalAlign: "top" }}>
                  {CHANGE_VERB[status as keyof typeof CHANGE_VERB] ?? status}
                </td>
                <td style={{ overflowWrap: "anywhere" }}>
                  <span title={path}>{path}</span>
                  {typeof change.from === "string" && (
                    <Metadata>From {change.from}</Metadata>
                  )}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    );
  if (tool.name === "shell" || tool.name === "read_shell_output")
    return <RecordedOutput text={tool.result} />;
  return <RecordedRows tool={tool} />;
}

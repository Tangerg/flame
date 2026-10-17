import { ansiText, renderAnsi } from "./ansi";
import { RecordedOutput } from "./recorded-output";
import { useMemo, useState } from "react";
import type { CapturedToolResult, ComponentContext } from "@flame/plugin-sdk";
import { Button, Metadata, SearchField } from "@flame/ui";

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
  const recorded = useMemo(
    () =>
      rows(tool).map((row, index) => ({
        ...row,
        number: row.number ?? index + 1,
      })),
    [tool],
  );
  const matching = useMemo(
    () =>
      recorded.filter((row) =>
        `${row.location ?? ""}\n${ansiText(row.body)}`
          .toLowerCase()
          .includes(filter.toLowerCase()),
      ),
    [recorded, filter],
  );
  const numberWidth = `${String(recorded.at(-1)?.number ?? 1).length}ch`;
  return (
    <div
      data-slot="recorded-result"
      style={{
        display: "flex",
        flexDirection: "column",
        gap: "var(--layout-control-gap)",
        minWidth: 0,
        maxHeight: "min(50vh, 24rem)",
      }}
    >
      <div data-slot="recorded-result-controls" style={{ flexShrink: 0 }}>
        <SearchField
          aria-label="Filter recorded result"
          placeholder="Find in recorded results"
          value={filter}
          onChange={(event) => {
            setFilter(event.target.value);
            setVisible(40);
          }}
        />
      </div>
      <div
        data-slot="recorded-result-viewport"
        role="region"
        aria-label="Recorded result rows"
        tabIndex={0}
        data-focus-inset=""
        style={{
          minWidth: 0,
          minHeight: 0,
          overflow: "auto",
          overscrollBehavior: "contain",
          scrollPaddingBlock: "var(--layout-control-gap)",
        }}
      >
        {matching.slice(0, visible).map((row, index) => (
          <div
            key={index}
            style={{
              display: "grid",
              gridTemplateColumns:
                row.location === undefined
                  ? `${numberWidth} minmax(0, 1fr)`
                  : "minmax(0, 1fr)",
              columnGap: "var(--layout-content-gap)",
              rowGap: "var(--layout-label-gap)",
              paddingBlock:
                row.location === undefined
                  ? undefined
                  : "var(--layout-row-padding)",
              minWidth: 0,
              fontFamily: "var(--font-mono)",
              fontSize: "var(--fs-code)",
              lineHeight: "var(--leading-relaxed)",
            }}
          >
            {row.location === undefined ? (
              <span
                style={{
                  color: "var(--color-text-faint)",
                  textAlign: "end",
                  userSelect: "none",
                  fontVariantNumeric: "tabular-nums",
                }}
              >
                <span className="sr-only">Line </span>
                {row.number}
              </span>
            ) : (
              <Metadata style={{ whiteSpace: "pre-wrap" }}>
                {row.location}
              </Metadata>
            )}
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
          </div>
        ))}
        {!matching.length && (
          <p>No matching recorded rows. Clear the filter to see all results.</p>
        )}
        {matching.length > visible && (
          <Button
            onClick={() => setVisible((current) => current + 40)}
            style={{ marginBlockStart: "var(--layout-control-gap)" }}
          >
            Show {Math.min(40, matching.length - visible)} more matching rows
          </Button>
        )}
      </div>
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

import { useCallback, useEffect, useState } from "react";
import type {
  ComponentContext,
  TelemetrySnapshot,
  SpanRow,
  MetricRow,
  LogRow,
} from "@flame/plugin-sdk";
import { Activity } from "@flame/icons/react";
import {
  Button,
  Panel,
  PanelTitle,
  CodeBlock,
  Disclosure,
  EmptyState,
  Facts,
  Metadata,
  RecordGroup,
  Select,
  Toolbar,
  type Fact,
} from "@flame/ui";
type Signal = "spans" | "metrics" | "logs";
type Observation = SpanRow | MetricRow | LogRow;

function title(row: Observation): string {
  if ("durationMillis" in row)
    return `${row.name} · ${row.durationMillis} ms · ${row.status}`;
  if ("severity" in row) return `${row.severity} · ${row.body}`;
  return `${row.name} [${row.kind}]`;
}

export function DiagnosticsReport({
  snapshot,
}: {
  snapshot: TelemetrySnapshot;
}) {
  const [signal, setSignal] = useState("spans");
  const rows = snapshot[signal as Signal];
  return (
    <>
      <Toolbar label="Telemetry filters">
        <Select
          id="signal"
          label="Telemetry signal"
          value={signal}
          onValueChange={setSignal}
          options={[
            { value: "spans", label: "Traces" },
            { value: "metrics", label: "Metrics" },
            { value: "logs", label: "Logs" },
          ]}
        />
      </Toolbar>
      <Metadata>
        {rows.length} observations in the last buffer read. Older observations
        may have been evicted.
      </Metadata>
      <RecordGroup id="output" aria-label="Local observations">
        {rows.map((row) => (
          <Disclosure key={`${signal}:${row.id}`} title={title(row)}>
            {"count" in row && (
              <Facts
                values={(["count", "sum", "p50", "p95", "avg"] as const)
                  .filter((key) => row[key] !== undefined)
                  .map(
                    (key) =>
                      [
                        key,
                        `${row[key]}${row.unit ? ` ${row.unit}` : ""}`,
                      ] as Fact,
                  )}
              />
            )}
            <CodeBlock label="Local observation details">
              {JSON.stringify(row, null, 2)}
            </CodeBlock>
          </Disclosure>
        ))}
        {!rows.length && (
          <EmptyState
            icon={Activity}
            size="compact"
            title="No local observations in this read."
            sub="Select another signal or refresh after using the workbench."
          />
        )}
      </RecordGroup>
      <Metadata>
        Refresh reads the current bounded client buffer. Use Clear above to
        remove local observations.
      </Metadata>
    </>
  );
}
type Read = { snapshot?: TelemetrySnapshot; error?: string };

export function Diagnostics({ context }: { context: ComponentContext }) {
  const [read, setRead] = useState<Read>({});
  const refresh = useCallback(() => {
    try {
      context.signal.throwIfAborted();
      setRead({ snapshot: context.telemetry.read() });
    } catch (error) {
      setRead((previous) => ({
        ...previous,
        error:
          error instanceof Error
            ? error.message
            : "Local observations could not be read.",
      }));
    }
  }, [context]);
  useEffect(refresh, [refresh]);
  function clear() {
    try {
      context.signal.throwIfAborted();
      context.telemetry.clear();
      setRead({});
      refresh();
    } catch (error) {
      setRead((previous) => ({
        ...previous,
        error:
          error instanceof Error
            ? error.message
            : "Local observations could not be cleared.",
      }));
    }
  }
  return (
    <Panel label="Client diagnostics">
      <Toolbar label="Client telemetry actions">
        <PanelTitle>Client diagnostics</PanelTitle>
        <Button disabled={context.signal.aborted} onClick={refresh}>
          Refresh
        </Button>
        <Button
          disabled={context.signal.aborted}
          variant="ghost"
          onClick={clear}
          title="Clear this client's telemetry buffer"
        >
          Clear local observations
        </Button>
      </Toolbar>
      <Metadata>
        Local traces, metrics and logs from this client. Runtime accounting and
        remote telemetry have their own owners.
      </Metadata>
      {read.error && <p role="alert">{read.error}</p>}
      {read.snapshot && <DiagnosticsReport snapshot={read.snapshot} />}
    </Panel>
  );
}

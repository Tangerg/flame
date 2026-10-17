import { useCallback, useEffect, useState } from "react";
import type {
  ComponentContext,
  TelemetrySnapshot,
  SpanRow,
  MetricRow,
  LogRow,
} from "@flame/plugin-sdk";
import { Activity, Refresh, Gauge, FileText } from "@flame/icons/react";
import {
  Panel,
  PanelHeader,
  PanelSection,
  ActionMenu,
  Segmented,
  IconButton,
  Stats,
  CodeBlock,
  Disclosure,
  EmptyState,
  Facts,
  Metadata,
  RecordGroup,
  RecordSummary,
  RecordBody,
  Badge,
  type Fact,
} from "@flame/ui";
type Signal = "spans" | "metrics" | "logs";
type Observation = SpanRow | MetricRow | LogRow;

function title(row: Observation) {
  if ("durationMillis" in row)
    return (
      <RecordSummary
        icon={Activity}
        title={row.name}
        description={`${row.durationMillis} ms · ${row.kind}`}
        status={
          <Badge tone={row.status === "error" ? "negative" : "neutral"}>
            {row.status}
          </Badge>
        }
      />
    );
  if ("severity" in row)
    return (
      <RecordSummary
        icon={FileText}
        title={row.body}
        description={
          <time dateTime={new Date(row.timeMs).toISOString()}>
            {new Intl.DateTimeFormat(navigator.language, {
              timeStyle: "medium",
            }).format(new Date(row.timeMs))}
          </time>
        }
        status={<Badge>{row.severity}</Badge>}
      />
    );
  return (
    <RecordSummary
      icon={Gauge}
      title={row.name}
      description={row.description || row.kind}
      status={<Badge>{row.kind}</Badge>}
    />
  );
}

function DiagnosticsReport({ snapshot }: { snapshot: TelemetrySnapshot }) {
  const [signal, setSignal] = useState("spans");
  const rows = snapshot[signal as Signal];
  return (
    <>
      <Stats
        aria-label="Local telemetry counts"
        values={[
          ["Traces", snapshot.spans.length],
          ["Metrics", snapshot.metrics.length],
          ["Logs", snapshot.logs.length],
        ]}
      />
      <PanelSection
        title="Local observations"
        description={`${rows.length} observations in the last buffer read. Older observations may have been evicted.`}
        actions={
          <Segmented
            ariaLabel="Telemetry signal"
            value={signal}
            onChange={setSignal}
            options={[
              { value: "spans", label: "Traces" },
              { value: "metrics", label: "Metrics" },
              { value: "logs", label: "Logs" },
            ]}
          />
        }
      >
        <RecordGroup variant="list" id="output" aria-label="Local observations">
          {rows.map((row) => (
            <Disclosure
              variant="row"
              key={`${signal}:${row.id}`}
              title={title(row)}
            >
              {"durationMillis" in row && (
                <>
                  {row.statusMessage && (
                    <RecordBody>{row.statusMessage}</RecordBody>
                  )}
                  <Facts
                    values={[
                      ["Started", new Date(row.startMs).toLocaleString()],
                      ["Duration", `${row.durationMillis} ms`],
                      ["Trace", row.traceId],
                      ["Parent span", row.parentSpanId],
                    ]}
                  />
                </>
              )}
              {"severity" in row && (
                <Facts
                  values={[
                    ["Recorded", new Date(row.timeMs).toLocaleString()],
                    ["Trace", row.traceId],
                    ["Span", row.spanId],
                  ]}
                />
              )}
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
              {Object.keys(row.attrs).length > 0 && (
                <PanelSection title="Attributes">
                  <Facts
                    values={Object.entries(row.attrs).map(([key, value]) => [
                      key,
                      String(value),
                    ])}
                  />
                </PanelSection>
              )}
              <Disclosure title="Raw observation">
                <CodeBlock label="Local observation details">
                  {JSON.stringify(row, null, 2)}
                </CodeBlock>
              </Disclosure>
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
      </PanelSection>
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
      <PanelHeader
        title="Client diagnostics"
        description="Local traces, metrics and logs from this workbench."
        actions={
          <>
            <IconButton
              icon={Refresh}
              aria-label="Refresh"
              size="md"
              disabled={context.signal.aborted}
              onClick={refresh}
            />
            <ActionMenu
              label="Diagnostic actions"
              disabled={context.signal.aborted}
              items={[
                {
                  label: "Clear local observations",
                  tone: "negative",
                  onSelect: clear,
                },
              ]}
            />
          </>
        }
      />
      {read.error && <p role="alert">{read.error}</p>}
      {read.snapshot && <DiagnosticsReport snapshot={read.snapshot} />}
      <Disclosure title="About client diagnostics">
        <Metadata>
          Only this client's local buffer is shown. Refresh reads its latest
          contents; Clear removes local observations. Runtime accounting and
          remote telemetry are unchanged.
        </Metadata>
      </Disclosure>
    </Panel>
  );
}

import { useState } from "react";
import { Activity } from "@flame/icons/react";
import {
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
import {
  mountView,
  ReadStatus,
  useRead,
  ViewPanel,
  type ViewProps,
} from "../../src/view";

type Observation = Record<string, unknown> & { id: string };
type Signal = "spans" | "metrics" | "logs";
type DiagnosticsSnapshot = {
  format: "flame.client-diagnostics.v1";
  source: "currentClient";
} & Record<Signal, { rows: Observation[]; omittedFromRead: number }>;
function validate(value: DiagnosticsSnapshot): void {
  if (
    value?.format !== "flame.client-diagnostics.v1" ||
    value.source !== "currentClient" ||
    !["spans", "metrics", "logs"].every((key) => {
      const section = value[key as Signal];
      return (
        Array.isArray(section?.rows) &&
        Number.isSafeInteger(section.omittedFromRead) &&
        section.omittedFromRead >= 0
      );
    })
  )
    throw new Error("Unsupported current-client diagnostics format.");
}
function Diagnostics({ view }: ViewProps<DiagnosticsSnapshot>) {
  const { data: snapshot } = useRead(view);
  const [signal, setSignal] = useState("spans");
  const section = snapshot[signal as Signal];
  return (
    <ViewPanel
      view={view}
      title="Client diagnostics"
      description="Local traces, metrics and logs from this graphical client. Runtime accounting and remote telemetry are separate facts."
    >
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
      <ReadStatus
        view={view}
        ready={`${section.rows.length} observations shown; ${section.omittedFromRead} omitted from the current buffer read. Older observations may already have left the bounded client buffer.`}
        loading="Reading local observations…"
      />
      <RecordGroup id="output" aria-label="Local observations">
        {section.rows.map((row) => (
          <Disclosure
            key={`${signal}:${row.id}`}
            title={
              signal === "spans"
                ? `${row.name} · ${row.durationMillis} ms · ${row.status}`
                : signal === "logs"
                  ? `${row.severity} · ${row.body}`
                  : `${row.name} [${row.kind}]`
            }
          >
            {signal === "metrics" && (
              <Facts
                values={["count", "sum", "p50", "p95", "avg"]
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
        {!section.rows.length && (
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
    </ViewPanel>
  );
}
mountView(Diagnostics, validate);

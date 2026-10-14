import { useCallback, useEffect, useRef, useState } from "react";
import type { ComponentContext } from "@flame/plugin-sdk";
import { ChartColumn } from "@flame/icons/react";
import type { UsageBucket, UsageSummary } from "@flame/runtime-contract/wire";
import {
  Button,
  Panel,
  PanelTitle,
  DataTable,
  EmptyState,
  Metadata,
  SearchField,
  Select,
  Stats,
  Toolbar,
} from "@flame/ui";

const number = new Intl.NumberFormat("en-US");
const dollars = new Intl.NumberFormat("en-US", {
  minimumFractionDigits: 2,
  maximumFractionDigits: 6,
});
const fields = [
  ["Input tokens", "inputTokens"],
  ["Output tokens", "outputTokens"],
  ["Cache read tokens", "cacheReadTokens"],
  ["Cache write tokens", "cacheWriteTokens"],
  ["Reasoning tokens", "reasoningTokens"],
] as const;
const options = [
  { value: "byProvider", label: "Provider" },
  { value: "byModel", label: "Model" },
  { value: "byDay", label: "UTC day" },
  { value: "byOrigin", label: "Calling origin" },
  { value: "byScope", label: "Association" },
];
const cost = (amount?: number) =>
  amount === undefined ? "Not reported" : dollars.format(amount);
export function UsageReport({ report }: { report: UsageSummary }) {
  const [group, setGroup] = useState("byProvider");
  const [filter, setFilter] = useState("");
  const buckets =
    report[
      group as "byProvider" | "byModel" | "byDay" | "byOrigin" | "byScope"
    ] ?? [];
  const matching = buckets.filter((bucket) =>
    bucket.key.toLocaleLowerCase().includes(filter.toLocaleLowerCase()),
  );
  const rows = matching.map((bucket: UsageBucket) => [
    bucket.key,
    cost(bucket.recordedCostUsd),
    `${bucket.coverage.reportedCalls}/${bucket.coverage.calls}`,
    `${bucket.coverage.pricedCalls}/${bucket.coverage.calls}`,
    ...fields.map(([, key]) => number.format(bucket.total[key] ?? 0)),
  ]);
  return (
    <>
      <Stats
        id="totals"
        aria-label="Recorded usage totals"
        values={[
          ["Known price subtotal (USD)", cost(report.recordedCostUsd)],
          ["Total cost (USD)", cost(report.total.costUsd)],
          ...fields.map(
            ([label, key]) =>
              [
                `Known ${label.toLowerCase()}`,
                number.format(report.total[key] ?? 0),
              ] as const,
          ),
          ["Model attempts", number.format(report.coverage.calls)],
          [
            "Usage recorded",
            `${report.coverage.reportedCalls}/${report.coverage.calls}`,
          ],
          [
            "Priced attempts",
            `${report.coverage.pricedCalls}/${report.coverage.calls}`,
          ],
          ["In progress", number.format(report.coverage.startedCalls)],
          ["Outcome unknown", number.format(report.coverage.unknownCalls)],
          ["Associated Sessions", number.format(report.sessions)],
          ["Associated Runs", number.format(report.runs)],
        ]}
      />
      <div id="empty" hidden={report.coverage.calls !== 0}>
        <EmptyState
          icon={ChartColumn}
          size="compact"
          title="No retained model attempts in this period."
          sub="Choose another period above or refresh after model activity."
        />
      </div>
      <section
        id="breakdown"
        aria-label="Usage breakdown"
        hidden={report.coverage.calls === 0}
      >
        <Toolbar label="Usage filters">
          <Select
            label="Group by"
            value={group}
            onValueChange={setGroup}
            options={options}
          />
          <SearchField
            id="filter"
            aria-label="Filter"
            placeholder="Provider or model"
            value={filter}
            onChange={(event) => setFilter(event.target.value)}
          />
        </Toolbar>
        <DataTable
          regionLabel="Scrollable usage breakdown"
          caption={`Reported usage by ${options.find((option) => option.value === group)!.label.toLowerCase()}`}
          columns={[
            { label: "Source" },
            ...[
              "Known cost (USD)",
              "Usage / attempts",
              "Priced / attempts",
              "Input",
              "Output",
              "Cache read",
              "Cache write",
              "Reasoning",
            ].map((label) => ({ label, numeric: true })),
          ]}
          rows={rows}
        />
        {!matching.length && (
          <EmptyState
            size="compact"
            title="No matching rows."
            sub="Clear the filter to show recorded usage."
          />
        )}
        <Metadata>
          Token totals include reported usage only. Unknown outcomes and
          unreported usage may add consumption. The known price subtotal
          includes priced attempts only; total cost requires every attempt to be
          priced. Filtering rows does not change the report. Deleted or never
          recorded calls cannot be reconstructed.
        </Metadata>
      </section>
      <Metadata>
        Zero tokens are recorded values. “Not reported” means Runtime has no
        complete price for that total or bucket. Child execution is counted
        through its owning Run; detached utility-model work is excluded.
      </Metadata>
    </>
  );
}

const periods = [
  { value: "all", label: "All time", query: {} },
  { value: "30", label: "Last 30 days", query: { sinceDays: 30 } },
  { value: "7", label: "Last 7 days", query: { sinceDays: 7 } },
] as const;
type Period = (typeof periods)[number];
type Read = {
  loading: boolean;
  accepted?: { report: UsageSummary; period: Period };
  error?: string;
};

export function Usage({ context }: { context: ComponentContext }) {
  const [period, setPeriod] = useState<Period>(periods[0]);
  const [read, setRead] = useState<Read>({ loading: true });
  const reading = useRef<AbortController>(undefined);
  const refresh = useCallback(async () => {
    reading.current?.abort();
    const controller = new AbortController();
    reading.current = controller;
    const signal = AbortSignal.any([context.signal, controller.signal]);
    setRead((previous) => ({ ...previous, loading: true, error: undefined }));
    try {
      const report = await context.track(
        context.runtime.usage.summary(period.query, signal),
      );
      if (!signal.aborted)
        setRead({ loading: false, accepted: { report, period } });
    } catch (error) {
      if (!signal.aborted)
        setRead((previous) => ({
          ...previous,
          loading: false,
          error:
            error instanceof Error
              ? error.message
              : "The report could not be read.",
        }));
    }
  }, [context, period]);
  useEffect(() => {
    void refresh();
    return () => reading.current?.abort();
  }, [refresh]);
  return (
    <Panel label="Usage">
      <Toolbar label="Usage report actions">
        <PanelTitle>Usage</PanelTitle>
        <Select
          label="Period"
          value={period.value}
          options={periods.map(({ value, label }) => ({ value, label }))}
          onValueChange={(value) =>
            setPeriod(periods.find((item) => item.value === value)!)
          }
        />
        <Button disabled={read.loading} onClick={() => void refresh()}>
          Refresh
        </Button>
      </Toolbar>
      <Metadata>
        Retained model attempts in the selected period, including auxiliary and
        unattached calls.
      </Metadata>
      {read.loading && <p role="status">Reading usage…</p>}
      {read.error && (
        <p role="alert">The current report could not be read: {read.error}</p>
      )}
      {read.accepted && (
        <>
          <p role="status">
            Recorded usage loaded. {read.accepted.period.label}.
            {read.accepted.period !== period &&
              " Showing the last accepted period while the selected period is unavailable."}
          </p>
          <UsageReport report={read.accepted.report} />
        </>
      )}
    </Panel>
  );
}

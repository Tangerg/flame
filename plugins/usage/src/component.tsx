import { useCallback, useEffect, useRef, useState } from "react";
import type { ComponentContext } from "@flame/plugin-sdk";
import { ChartColumn, Refresh } from "@flame/icons/react";
import type { UsageSummary } from "@flame/runtime-contract/wire";
import {
  Panel,
  PanelHeader,
  PanelSection,
  ListRow,
  RecordHeading,
  RecordMeta,
  Facts,
  ShareBar,
  MetricValue,
  IconButton,
  Disclosure,
  RecordGroup,
  EmptyState,
  Metadata,
  SearchField,
  FilterBar,
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
  return (
    <>
      <PanelSection
        title="Overview"
        description={`${number.format(report.coverage.calls)} model attempts · ${number.format(report.coverage.reportedCalls)} with usage · ${number.format(report.coverage.pricedCalls)} priced`}
      >
        <Stats
          id="totals"
          aria-label="Recorded usage totals"
          values={[
            report.total.costUsd === undefined
              ? ["Known price subtotal (USD)", cost(report.recordedCostUsd)]
              : ["Total cost (USD)", cost(report.total.costUsd)],
            [
              "Known input tokens",
              number.format(report.total.inputTokens ?? 0),
            ],
            [
              "Known output tokens",
              number.format(report.total.outputTokens ?? 0),
            ],
          ]}
        />
        <Disclosure title="Accounting details">
          <Facts
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
          <Metadata>
            Token totals include reported usage only. Unknown outcomes and
            unreported usage may add consumption. The known price subtotal
            includes priced attempts only; total cost requires every attempt to
            be priced. Deleted or never recorded calls cannot be reconstructed.
            Zero tokens are recorded values. “Not reported” means there is no
            complete price for that total or bucket.
          </Metadata>
        </Disclosure>
      </PanelSection>
      <div id="empty" hidden={report.coverage.calls !== 0}>
        <EmptyState
          icon={ChartColumn}
          size="compact"
          title="No retained model attempts in this period."
          sub="Choose another period above or refresh after model activity."
        />
      </div>
      <PanelSection
        id="breakdown"
        aria-label="Usage breakdown"
        hidden={report.coverage.calls === 0}
        title="Usage breakdown"
        description={`Recorded costs by ${options.find((option) => option.value === group)!.label.toLowerCase()}`}
      >
        <FilterBar
          label="Usage filters"
          search={
            <SearchField
              size="lg"
              id="filter"
              aria-label="Filter"
              placeholder="Find a source"
              value={filter}
              onChange={(event) => setFilter(event.target.value)}
            />
          }
        >
          <Select
            size="lg"
            label="Group by"
            value={group}
            onValueChange={setGroup}
            options={options}
          />
        </FilterBar>
        <RecordGroup variant="list" aria-label="Recorded usage by source">
          {matching.map((bucket) => (
            <ListRow key={bucket.key}>
              <RecordHeading
                title={bucket.key}
                actions={
                  <MetricValue>{cost(bucket.recordedCostUsd)} USD</MetricValue>
                }
                description={
                  <RecordMeta>
                    <span>{number.format(bucket.coverage.calls)} attempts</span>
                    <span>
                      {number.format(bucket.total.inputTokens ?? 0)} known input
                    </span>
                    <span>
                      {number.format(bucket.total.outputTokens ?? 0)} known
                      output tokens
                    </span>
                  </RecordMeta>
                }
              />
              {bucket.recordedCostUsd !== undefined &&
                report.recordedCostUsd !== undefined &&
                report.recordedCostUsd > 0 && (
                  <ShareBar
                    value={bucket.recordedCostUsd}
                    total={report.recordedCostUsd}
                  />
                )}
              <Disclosure title="Token and coverage details">
                <Facts
                  values={[
                    ["Known cost (USD)", cost(bucket.recordedCostUsd)],
                    [
                      "Usage / attempts",
                      `${bucket.coverage.reportedCalls}/${bucket.coverage.calls}`,
                    ],
                    [
                      "Priced / attempts",
                      `${bucket.coverage.pricedCalls}/${bucket.coverage.calls}`,
                    ],
                    ...fields.map(
                      ([label, key]) =>
                        [
                          `Known ${label.toLowerCase()}`,
                          number.format(bucket.total[key] ?? 0),
                        ] as const,
                    ),
                  ]}
                />
              </Disclosure>
            </ListRow>
          ))}
          {!matching.length && (
            <EmptyState
              size="compact"
              title="No matching rows."
              sub="Clear the filter to show recorded usage."
            />
          )}
        </RecordGroup>
        <Metadata>Filtering rows does not change the report totals.</Metadata>
      </PanelSection>
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
      <PanelHeader
        title="Usage"
        description="Recorded model usage and cost."
        actions={
          <>
            <Toolbar label="Usage report actions">
              <Select
                label="Period"
                value={period.value}
                options={periods.map(({ value, label }) => ({ value, label }))}
                onValueChange={(value) =>
                  setPeriod(periods.find((item) => item.value === value)!)
                }
              />
            </Toolbar>
            <IconButton
              icon={Refresh}
              aria-label="Refresh"
              size="md"
              disabled={read.loading}
              onClick={() => void refresh()}
            />
          </>
        }
      />

      {read.loading && <p role="status">Reading usage…</p>}
      {read.error && (
        <p role="alert">The current report could not be read: {read.error}</p>
      )}
      {read.accepted && (
        <>
          <Metadata
            role="status"
            className={read.accepted.period === period ? "sr-only" : undefined}
          >
            Recorded usage loaded. {read.accepted.period.label}.
            {read.accepted.period !== period &&
              " Showing the last accepted period while the selected period is unavailable."}
          </Metadata>
          <UsageReport report={read.accepted.report} />
        </>
      )}
    </Panel>
  );
}

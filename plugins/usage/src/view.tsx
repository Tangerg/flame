import { useState } from "react";
import { ChartColumn } from "@flame/icons/react";
import type { UsageBucket, UsageSummary } from "@flame/runtime-contract/wire";
import {
  DataTable,
  EmptyState,
  Metadata,
  SearchField,
  Select,
  Stats,
  Toolbar,
} from "@flame/ui";
import {
  mountView,
  ReadStatus,
  useRead,
  ViewPanel,
  type ViewProps,
} from "../../src/view";

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
];
const cost = (amount?: number) =>
  amount === undefined ? "Not reported" : dollars.format(amount);
function Usage({ view }: ViewProps<UsageSummary>) {
  const { data: report } = useRead(view);
  const [group, setGroup] = useState("byProvider");
  const [filter, setFilter] = useState("");
  const buckets = report[group as "byProvider" | "byModel" | "byDay"] ?? [];
  const matching = buckets.filter((bucket) =>
    bucket.key.toLocaleLowerCase().includes(filter.toLocaleLowerCase()),
  );
  const rows = matching.map((bucket: UsageBucket) => [
    bucket.key,
    cost(bucket.costUsd),
    number.format(bucket.runs ?? 0),
    ...fields.map(([, key]) => number.format(bucket[key] ?? 0)),
  ]);
  return (
    <ViewPanel
      view={view}
      title="Usage"
      description="Recorded usage from finished Runs in the selected period."
    >
      <ReadStatus
        view={view}
        ready="Recorded usage loaded."
        loading="Reading usage…"
      />
      <Stats
        id="totals"
        aria-label="Recorded usage totals"
        values={[
          ["Recorded cost (USD)", cost(report.total.costUsd)],
          ...fields.map(
            ([label, key]) =>
              [label, number.format(report.total[key] ?? 0)] as const,
          ),
          ["Sessions", number.format(report.sessions ?? 0)],
          ["Finished Runs", number.format(report.runs ?? 0)],
        ]}
      />
      <div id="empty" hidden={(report.runs ?? 0) !== 0}>
        <EmptyState
          icon={ChartColumn}
          size="compact"
          title="No finished Runs with reported usage in this period."
          sub="Choose another period above or refresh after a Run finishes."
        />
      </div>
      <section
        id="breakdown"
        aria-label="Usage breakdown"
        hidden={(report.runs ?? 0) === 0}
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
              "Cost (USD)",
              "Runs",
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
          Scroll horizontally to read every token column. Filtering rows does
          not change recorded totals.
        </Metadata>
      </section>
      <Metadata>
        Zero tokens are recorded values. “Not reported” means Runtime has no
        complete price for that total or bucket. Child execution is counted
        through its owning Run; detached utility-model work is excluded.
      </Metadata>
    </ViewPanel>
  );
}
mountView(Usage);

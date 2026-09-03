import type { ReactNode } from "react";
import { useState } from "react";
import { EmptyState, ProviderIcon, Segmented, Surface } from "@/ui";
import { fmtCost, fmtTokens } from "@/lib/format";
import { useT } from "@/lib/i18n";
import {
  USAGE_RANGES,
  UsageRange,
  type UsageBreakdownBucket,
  usagePeriodForRange,
  usageTokens,
  useUsageReport,
} from "../application/usageConfig";

function BreakdownSection({
  title,
  buckets,
  icon,
}: {
  title: string;
  buckets: UsageBreakdownBucket[];
  icon?: (key: string) => ReactNode;
}) {
  if (buckets.length === 0) return null;
  return (
    <Surface>
      <div className="mb-1.5 text-ui-md font-medium text-fg-muted">{title}</div>
      <div className="flex flex-col">
        {buckets.map((b) => (
          <div
            key={b.key}
            className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 rounded-md px-2 py-2 transition-colors hover:bg-hover"
          >
            <div className="flex min-w-0 items-center gap-2">
              {icon?.(b.key)}
              <span className="truncate text-ui-md text-fg">{b.key}</span>
            </div>
            <div className="flex items-center gap-3 font-mono text-ui-md tabular-nums">
              <span className="text-fg-muted">{fmtTokens(usageTokens(b))}</span>
              {b.costUsd !== undefined && (
                <span className="w-16 text-right text-fg">{fmtCost(b.costUsd)}</span>
              )}
            </div>
          </div>
        ))}
      </div>
    </Surface>
  );
}

/** The one card that answers "what has this cost" — a headline figure and the token lines
 *  that only appear when the provider reported them. Its own component because the pane
 *  around it is a four-state machine, and the optional metrics are not part of that. */
function UsageTotals({
  total,
  sessions,
  runs,
}: {
  total: NonNullable<ReturnType<typeof useUsageReport>["data"]>["total"];
  sessions: number;
  runs: number;
}) {
  const t = useT();
  return (
    <Surface className="flex flex-col gap-2">
      <div className="flex items-baseline justify-between gap-3">
        <span className="text-ui-md font-medium text-fg-muted">{t("usage.total")}</span>
        <span className="font-mono text-display-md font-semibold tabular-nums text-fg">
          {total.costUsd !== undefined ? fmtCost(total.costUsd) : "—"}
        </span>
      </div>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 font-mono text-ui-md tabular-nums text-fg-muted">
        <span>↑{fmtTokens(total.inputTokens ?? 0)}</span>
        <span>↓{fmtTokens(total.outputTokens ?? 0)}</span>
        {(total.cacheReadTokens ?? 0) > 0 && (
          <span className="text-fg-faint">
            {t("usage.cache")} {fmtTokens(total.cacheReadTokens ?? 0)}
          </span>
        )}
        {(total.cacheWriteTokens ?? 0) > 0 && (
          <span className="text-fg-faint">
            {t("usage.cacheWrite")} {fmtTokens(total.cacheWriteTokens ?? 0)}
          </span>
        )}
        {(total.reasoningTokens ?? 0) > 0 && (
          <span className="text-fg-faint">
            {t("usage.reasoning")} {fmtTokens(total.reasoningTokens ?? 0)}
          </span>
        )}
        <span className="text-fg-faint">
          · {t("usage.sessions", { count: sessions })} · {t("usage.runs", { count: runs })}
        </span>
      </div>
    </Surface>
  );
}

export function UsagePane() {
  const t = useT();
  const [range, setRange] = useState<UsageRange>(UsageRange.AllTime);
  const { data, isLoading, isError } = useUsageReport(usagePeriodForRange(range));

  const total = data?.total;
  const totalTokens = total ? usageTokens(total) : 0;
  const hasSpend = totalTokens > 0 || (total?.costUsd ?? 0) > 0;

  return (
    <div className="flex flex-col gap-4">
      <div className="self-end">
        <Segmented
          value={range}
          options={USAGE_RANGES.map((item) => ({ value: item.value, label: t(item.label) }))}
          onChange={setRange}
          ariaLabel={t("usage.rangeAria")}
        />
      </div>

      {isLoading && <div className="text-ui-md text-fg-muted">{t("usage.loading")}</div>}
      {isError && <div className="text-ui-md text-negative">{t("usage.error")}</div>}

      {data && !hasSpend && (
        <EmptyState icon="chart" title={t("usage.empty")} sub={t("usage.empty.sub")} />
      )}

      {data && hasSpend && (
        <>
          <UsageTotals total={data.total} sessions={data.sessions ?? 0} runs={data.runs ?? 0} />

          <BreakdownSection
            title={t("usage.byProvider")}
            buckets={data.byProvider ?? []}
            icon={(key) => <ProviderIcon provider={key} size="md" />}
          />
          <BreakdownSection title={t("usage.byModel")} buckets={data.byModel ?? []} />
          <BreakdownSection title={t("usage.byDay")} buckets={data.byDay ?? []} />
        </>
      )}
    </div>
  );
}

import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { useState } from "react";
import { EmptyState, ProviderIcon, Segmented, Surface } from "@/ui";
import { fmtCost, fmtTokens } from "@/lib/format";
import { useT } from "@/lib/i18n";
import {
  USAGE_RANGES,
  UsageRange,
  usagePeriodForRange,
  usageTokens,
  useUsageReport,
} from "../application/usageConfig";
import type { UsageAmount, UsageBucket } from "../application/ports/usageGateway";
import { color, face, space, type as typeStep, weight } from "@/styles/tokens.stylex";
import { settingStyles as ss } from "../../kit/settingStyles";

const u = stylex.create({
  ink: { color: color.fg },
  // A money column reads down, so it holds one measure and aligns on the right.
  cost: { width: space.s16, textAlign: "right", color: color.fg },
  totalLine: {
    display: "flex",
    alignItems: "baseline",
    justifyContent: "space-between",
    gap: space.s3,
  },
  total: {
    fontFamily: "var(--font-mono)",
    fontWeight: weight.semibold,
    color: color.fg,
  },
  breakdown: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    columnGap: space.s3,
    rowGap: space.s1,
    fontFamily: "var(--font-mono)",
    color: color.fgMuted,
  },
});

function BreakdownSection({
  title,
  buckets,
  icon,
}: {
  title: string;
  buckets: UsageBucket[];
  icon?: (key: string) => ReactNode;
}) {
  if (buckets.length === 0) return null;
  return (
    <Surface>
      <div {...stylex.props(ss.caption, typeStep.uiMd)}>{title}</div>
      <div {...stylex.props(ss.column)}>
        {buckets.map((b) => (
          <div key={b.key} {...stylex.props(ss.nameGrid, ss.hoverRow, ss.hoverRowTight)}>
            <div {...stylex.props(ss.line, ss.min)}>
              {icon?.(b.key)}
              <span {...stylex.props(ss.truncate, u.ink, typeStep.uiMd)}>{b.key}</span>
            </div>
            <div {...stylex.props(ss.lineWide, typeStep.uiMd, face.mono)}>
              <span {...stylex.props(ss.muted)}>{fmtTokens(usageTokens(b))}</span>
              {b.costUsd !== undefined && (
                <span {...stylex.props(u.cost, ss.figures)}>{fmtCost(b.costUsd)}</span>
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
  total: UsageAmount;
  sessions: number;
  runs: number;
}) {
  const t = useT();
  return (
    <Surface className={stylex.props(ss.stackTight).className}>
      <div {...stylex.props(u.totalLine)}>
        <span {...stylex.props(ss.captionInline, typeStep.uiMd)}>{t("usage.total")}</span>
        <span {...stylex.props(u.total, ss.figures)}>
          {total.costUsd !== undefined ? fmtCost(total.costUsd) : "—"}
        </span>
      </div>
      <div {...stylex.props(u.breakdown, typeStep.uiMd)}>
        <span>↑{fmtTokens(total.inputTokens ?? 0)}</span>
        <span>↓{fmtTokens(total.outputTokens ?? 0)}</span>
        {(total.cacheReadTokens ?? 0) > 0 && (
          <span {...stylex.props(ss.faint)}>
            {t("usage.cache")} {fmtTokens(total.cacheReadTokens ?? 0)}
          </span>
        )}
        {(total.cacheWriteTokens ?? 0) > 0 && (
          <span {...stylex.props(ss.faint)}>
            {t("usage.cacheWrite")} {fmtTokens(total.cacheWriteTokens ?? 0)}
          </span>
        )}
        {(total.reasoningTokens ?? 0) > 0 && (
          <span {...stylex.props(ss.faint)}>
            {t("usage.reasoning")} {fmtTokens(total.reasoningTokens ?? 0)}
          </span>
        )}
        <span {...stylex.props(ss.faint)}>
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
    <div {...stylex.props(ss.stackWide)}>
      <div {...stylex.props(ss.selfEnd)}>
        <Segmented
          value={range}
          options={USAGE_RANGES.map((item) => ({ value: item.value, label: t(item.label) }))}
          onChange={setRange}
          ariaLabel={t("usage.rangeAria")}
        />
      </div>

      {isLoading && <div {...stylex.props(ss.muted, typeStep.uiMd)}>{t("usage.loading")}</div>}
      {isError && <div {...stylex.props(ss.negative, typeStep.uiMd)}>{t("usage.error")}</div>}

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

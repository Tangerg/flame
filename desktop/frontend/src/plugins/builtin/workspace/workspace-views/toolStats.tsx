import type { ToolStat, ToolStatsSummary } from "../application/toolStats";
import { toolStats, toolTimeShare } from "../application/toolStats";
import { useActiveSessionToolCalls } from "@/plugins/builtin/agent/public/run";
import { Badge, EmptyState, Icon, ProgressBar, Sparkline, knownIconName } from "@/ui";
import { fmtDuration } from "@/lib/format";
import { useT } from "@/lib/i18n";
import { lookupExtensionByKey, TOOL_ICON } from "@/plugins/sdk";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";

export function ToolStatsTab() {
  const t = useT();
  const summary = toolStats(useActiveSessionToolCalls());

  return (
    <WorkspaceViewLayout
      icon="chart"
      titleStrong
      title="toolStats.title"
      sub={
        summary.calls > 0
          ? t("toolStats.summary", {
              calls: summary.calls,
              duration: fmtDuration(summary.totalMs),
            })
          : undefined
      }
      scrollClassName="py-1"
    >
      {summary.rows.length === 0 ? (
        <EmptyState
          icon="chart"
          title={t("toolStats.empty.title")}
          sub={t("toolStats.empty.sub")}
        />
      ) : (
        summary.rows.map((row) => <ToolStatRow key={row.name} row={row} summary={summary} />)
      )}
    </WorkspaceViewLayout>
  );
}

function ToolStatRow({ row, summary }: { row: ToolStat; summary: ToolStatsSummary }) {
  const t = useT();
  const icon = knownIconName(lookupExtensionByKey(TOOL_ICON, row.name)) ?? "lightning";

  return (
    <div className="px-[var(--density-column-gutter-wide)] py-2">
      <div className="flex min-w-0 items-baseline gap-2">
        <Icon name={icon} size="sm" className="shrink-0 self-center text-fg-muted" />
        <span className="min-w-0 flex-1 truncate text-ui-md text-fg">{row.name}</span>
        {row.failed > 0 && (
          <Badge tone="negative">{t("toolStats.failed", { n: row.failed })}</Badge>
        )}
        {row.denied > 0 && <Badge tone="warning">{t("toolStats.denied", { n: row.denied })}</Badge>}
        <span className="shrink-0 font-mono text-ui-xs tabular-nums text-fg-muted">
          {row.timed > 0 ? fmtDuration(row.totalMs) : "—"}
        </span>
      </div>
      <div className="mt-1 flex items-center gap-2.5">
        <ProgressBar
          value={toolTimeShare(row, summary) * 100}
          label={t("toolStats.share", { name: row.name })}
          className="h-1 flex-1"
        />
        {row.durations.length > 1 && (
          <Sparkline
            data={row.durations}
            label={t("toolStats.trend", { name: row.name })}
            className="shrink-0 text-fg-faint"
          />
        )}
        <span className="shrink-0 text-ui-sm text-fg-faint">
          {t("toolStats.calls", { n: row.calls })}
          {row.timed > 0 &&
            ` · ${t("toolStats.slowest", { duration: fmtDuration(row.slowestMs) })}`}
        </span>
      </div>
    </div>
  );
}

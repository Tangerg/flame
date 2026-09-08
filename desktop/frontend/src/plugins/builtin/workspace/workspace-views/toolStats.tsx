import * as stylex from "@stylexjs/stylex";
import type { ToolStat, ToolStatsSummary } from "../application/toolStats";
import { toolStats, toolTimeShare } from "../application/toolStats";
import { useActiveSessionToolCalls } from "@/plugins/builtin/agent/public/run";
import { Badge, EmptyState, Icon, ProgressBar, Sparkline, knownIconName } from "@/ui";
import { fmtDuration } from "@/lib/format";
import { useT } from "@/lib/i18n";
import { type as typeStep } from "@/styles/tokens.stylex";
import { viewStyles as vs } from "./views/viewStyles";
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
    <div {...stylex.props(vs.gutter, vs.rowPad)}>
      <div {...stylex.props(vs.lineBaseline)}>
        <Icon name={icon} size="sm" className={stylex.props(vs.glyphInline).className} />
        <span {...stylex.props(vs.fill, vs.truncate, typeStep.uiMd)}>{row.name}</span>
        {row.failed > 0 && (
          <Badge tone="negative">{t("toolStats.failed", { n: row.failed })}</Badge>
        )}
        {row.denied > 0 && <Badge tone="warning">{t("toolStats.denied", { n: row.denied })}</Badge>}
        <span {...stylex.props(vs.hold, vs.mono, vs.muted, typeStep.uiXs)}>
          {row.timed > 0 ? fmtDuration(row.totalMs) : "—"}
        </span>
      </div>
      <div {...stylex.props(vs.meterLine)}>
        <ProgressBar
          value={toolTimeShare(row, summary) * 100}
          label={t("toolStats.share", { name: row.name })}
          weight="row"
          className={stylex.props(vs.grow).className}
        />
        {row.durations.length > 1 && (
          <Sparkline
            data={row.durations}
            label={t("toolStats.trend", { name: row.name })}
            className={stylex.props(vs.hold, vs.caption).className}
          />
        )}
        <span {...stylex.props(vs.hold, vs.caption, typeStep.uiSm)}>
          {t("toolStats.calls", { n: row.calls })}
          {row.timed > 0 &&
            ` · ${t("toolStats.slowest", { duration: fmtDuration(row.slowestMs) })}`}
        </span>
      </div>
    </div>
  );
}

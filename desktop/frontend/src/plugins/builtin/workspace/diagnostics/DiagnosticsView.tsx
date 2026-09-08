import * as stylex from "@stylexjs/stylex";
import type { Tone } from "@/lib/tone";
import type { MetricRow } from "@/lib/observability/stores";
import { useTelemetryStore } from "@/lib/observability/stores";
import { useMemo, useState } from "react";
import { Button, Segmented, toneInk, vocab } from "@/ui";
import { Cell, Empty, Row, VirtualList } from "./primitives";
import { TracesPanel } from "./TracesPanel";
import { useT } from "@/lib/i18n";
import { color, space, surface, type as typeStep, weight } from "@/styles/tokens.stylex";

type Signal = "traces" | "metrics" | "logs";

const SIGNALS = [
  { value: "traces" as const, label: "diagnostics.signal.traces" },
  { value: "metrics" as const, label: "diagnostics.signal.metrics" },
  { value: "logs" as const, label: "diagnostics.signal.logs" },
];

// The panel's own table: two columns that hold a measure and one that takes what is left.
// The widths belong here, beside the header that names them, rather than at each `Cell`.
const logColumns = stylex.create({
  level: { width: space.s12 },
  message: { flexGrow: 1 },
  span: { width: "calc(var(--spacing) * 24)" },
});

const d = stylex.create({
  page: {
    display: "flex",
    height: "100%",
    flexDirection: "column",
    gap: space.s3,
    padding: space.s6,
  },
  masthead: { display: "flex", alignItems: "center", justifyContent: "space-between" },
  title: { color: color.fg, fontWeight: weight.semibold },
  subtitle: { marginTop: space.s0_5, color: color.fgMuted },
  controls: { display: "flex", alignItems: "center", gap: space.s2 },
  logRow: { minHeight: "calc(var(--spacing) * 7)" },
  mono: { fontFamily: "var(--font-mono)" },
  metricsScroller: {
    display: "grid",
    flex: 1,
    minHeight: 0,
    alignContent: "start",
    gap: space.s4,
    overflowY: "auto",
  },
  section: { display: "grid", gap: space.s1_5 },
  instrument: { fontFamily: "var(--font-mono)", fontWeight: weight.semibold, color: color.fg },
  kind: { marginInlineStart: space.s2, color: color.fgFaint },
  headCell: {
    paddingBlock: space.s1,
    paddingRight: space.s3,
    textAlign: "left",
    fontWeight: weight.medium,
  },
  cell: { paddingBlock: space.s0_5, paddingRight: space.s3 },
  // Columns of numbers read down, so they align on the right and hold one glyph width.
  figures: { textAlign: "right" },
  numeric: { textAlign: "right", fontVariantNumeric: "tabular-nums", color: color.fg },
  metricRow: {
    transitionProperty: "background-color",
    backgroundColor: { default: null, ":hover": surface.hover },
  },
});

export function DiagnosticsView() {
  const [signal, setSignal] = useState<Signal>("traces");
  const t = useT();
  const clear = useTelemetryStore((s) => s.clear);

  return (
    <div {...stylex.props(d.page)}>
      <div {...stylex.props(d.masthead)}>
        <div>
          <div {...stylex.props(d.title, typeStep.displaySm)}>{t("diagnostics.title")}</div>
          <div {...stylex.props(d.subtitle, typeStep.uiMd)}>{t("diagnostics.description")}</div>
        </div>
        <div {...stylex.props(d.controls)}>
          <Segmented
            value={signal}
            options={SIGNALS}
            onChange={setSignal}
            ariaLabel={t("diagnostics.signalAria")}
          />
          <Button variant="outline" size="sm" onClick={clear}>
            {t("diagnostics.clear")}
          </Button>
        </div>
      </div>

      {signal === "traces" && <TracesPanel />}
      {signal === "metrics" && <MetricsPanel />}
      {signal === "logs" && <LogsPanel />}
    </div>
  );
}

function LogsPanel() {
  const t = useT();
  const logs = useTelemetryStore((s) => s.logs);
  const ordered = useMemo(() => logs.slice().reverse(), [logs]);

  if (ordered.length === 0) return <Empty hint={t("diagnostics.empty.logs")} />;

  return (
    <VirtualList
      count={ordered.length}
      rowHeight={28}
      header={
        <Row head>
          <Cell styles={logColumns.level}>lvl</Cell>
          <Cell styles={logColumns.message}>message</Cell>
          <Cell styles={logColumns.span}>span</Cell>
        </Row>
      }
      renderRow={(i) => {
        const l = ordered[i]!;
        return (
          <Row styles={d.logRow}>
            <Cell styles={logColumns.level}>
              <span {...stylex.props(toneInk[severityTone(l.severity)])}>{l.severity}</span>
            </Cell>
            <Cell styles={logColumns.message}>
              <span {...stylex.props(vocab.truncate)}>{l.body}</span>
            </Cell>
            <Cell styles={logColumns.span}>
              <span {...stylex.props(vocab.faint)}>{l.spanId ? l.spanId.slice(0, 8) : "—"}</span>
            </Cell>
          </Row>
        );
      }}
    />
  );
}

// Map, not object: keyed off the telemetry stream, where `constructor` would answer with an
// inherited member instead of the fallback tone.
const SEVERITY_TONE = new Map<string, Tone>([
  ["ERROR", "negative"],
  ["WARN", "warning"],
  ["DEBUG", "neutral"],
]);

function severityTone(sev: string): Tone {
  return SEVERITY_TONE.get(sev) ?? "neutral";
}

function MetricsPanel() {
  const t = useT();
  const metrics = useTelemetryStore((s) => s.metrics);
  const grouped = useMemo(() => groupByName(Object.values(metrics)), [metrics]);

  if (grouped.length === 0) return <Empty hint={t("diagnostics.empty.metrics")} />;

  return (
    <div {...stylex.props(d.metricsScroller)}>
      {grouped.map((g) => (
        <InstrumentSection key={g.name} group={g} />
      ))}
    </div>
  );
}

interface NameGroup {
  name: string;
  unit: string;
  description: string;
  kind: MetricRow["kind"];
  rows: MetricRow[];
}

// Map, not object: grouped by instrument name, where `constructor` answers with a function
// that reads as an existing group and then throws on `g.rows.push`.
function groupByName(rows: MetricRow[]): NameGroup[] {
  const by = new Map<string, NameGroup>();
  for (const r of rows) {
    const g = by.get(r.name);
    if (g) g.rows.push(r);
    else
      by.set(r.name, {
        name: r.name,
        unit: r.unit,
        description: r.description,
        kind: r.kind,
        rows: [r],
      });
  }
  return [...by.values()]
    .map((g) => ({ ...g, rows: g.rows.slice().sort((a, b) => b.count - a.count) }))
    .sort((a, b) => a.name.localeCompare(b.name));
}

function InstrumentSection({ group }: { group: NameGroup }) {
  return (
    <section {...stylex.props(d.section)}>
      <header>
        <div {...stylex.props(d.instrument, typeStep.uiMd)}>
          {group.name}
          <span {...stylex.props(d.kind)}>[{group.kind}]</span>
        </div>
        {group.description && (
          <div {...stylex.props(d.subtitle, typeStep.uiSm)}>{group.description}</div>
        )}
      </header>
      <table {...stylex.props(typeStep.uiMd)}>
        <thead {...stylex.props(vocab.faint, typeStep.uiXs)}>
          <tr>
            <th {...stylex.props(d.headCell)}>attrs</th>
            <th {...stylex.props(d.headCell, d.figures)}>count</th>
            {group.kind === "histogram" && (
              <>
                <th {...stylex.props(d.headCell, d.figures)}>p50</th>
                <th {...stylex.props(d.headCell, d.figures)}>p95</th>
                <th {...stylex.props(d.headCell, d.figures)}>avg</th>
              </>
            )}
            <th {...stylex.props(d.headCell, d.figures)}>
              {group.kind === "histogram" ? "sum" : "value"}
            </th>
          </tr>
        </thead>
        <tbody {...stylex.props(d.mono)}>
          {group.rows.map((r) => (
            <tr key={r.id} {...stylex.props(d.metricRow)}>
              <td {...stylex.props(d.cell, vocab.muted)}>{formatAttrs(r.attrs)}</td>
              <td {...stylex.props(d.cell, d.numeric)}>{r.count}</td>
              {group.kind === "histogram" && (
                <>
                  <td {...stylex.props(d.cell, d.numeric)}>{fmt(r.p50, group.unit)}</td>
                  <td {...stylex.props(d.cell, d.numeric)}>{fmt(r.p95, group.unit)}</td>
                  <td {...stylex.props(d.cell, d.numeric)}>{fmt(r.avg, group.unit)}</td>
                </>
              )}
              <td {...stylex.props(d.cell, d.numeric)}>{fmt(r.sum, group.unit)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

function formatAttrs(attrs: Record<string, string | number | boolean>): string {
  const entries = Object.entries(attrs);
  if (entries.length === 0) return "—";
  return entries.map(([k, v]) => `${k}=${String(v)}`).join(" ");
}

function fmt(n: number | undefined, unit: string): string {
  if (n === undefined) return "—";
  const rounded = n < 10 ? n.toFixed(1) : Math.round(n).toString();
  return unit ? `${rounded} ${unit}` : rounded;
}

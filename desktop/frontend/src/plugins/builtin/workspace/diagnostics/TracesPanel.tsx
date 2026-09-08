import * as stylex from "@stylexjs/stylex";
import type { Tone } from "@/lib/tone";
import type { ReactNode } from "react";
import type { SpanRow } from "@/lib/observability/stores";
import { useTelemetryStore } from "@/lib/observability/stores";
import { Fragment, useCallback, useId, useMemo, useState } from "react";
import { useT } from "@/lib/i18n";
import { color, space, surface, type as typeStep } from "@/styles/tokens.stylex";
import { Icon, Pressable, Well, chevron, toneInk, vocab } from "@/ui";
import { Cell, Empty, Row, VirtualList } from "./primitives";

export function TracesPanel() {
  const t = useT();
  const spans = useTelemetryStore((s) => s.spans);
  const ordered = useMemo(() => spans.slice().reverse(), [spans]);
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(() => new Set());
  const toggle = useCallback((id: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  if (ordered.length === 0) return <Empty hint={t("diagnostics.empty.traces")} />;

  return (
    <VirtualList
      count={ordered.length}
      rowHeight={32}
      header={
        <Row head>
          <Cell styles={spanColumns.chevron} />
          <Cell styles={spanColumns.name}>span</Cell>
          <Cell styles={spanColumns.duration}>dur</Cell>
          <Cell styles={spanColumns.status}>status</Cell>
          <Cell styles={spanColumns.trace}>trace</Cell>
        </Row>
      }
      renderRow={(i) => {
        const s = ordered[i]!;
        return <SpanRowItem span={s} open={expanded.has(s.id)} onToggle={() => toggle(s.id)} />;
      }}
    />
  );
}

// This panel's columns, declared beside the header that names them.
const spanColumns = stylex.create({
  chevron: { width: space.s4 },
  name: { flexGrow: 1, minWidth: 0 },
  duration: { width: space.s16 },
  status: { width: space.s16 },
  trace: { width: "calc(var(--spacing) * 28)" },
});

const tr = stylex.create({
  spanRow: {
    display: "flex",
    minHeight: space.s8,
    width: "100%",
    alignItems: "center",
    gap: space.s3,
    backgroundColor: { default: "transparent", ":hover": surface.hover },
    paddingInline: space.s1,
    fontFamily: "var(--font-mono)",
    color: color.fg,
    transitionProperty: "background-color",
  },
  chevronBox: { display: "flex", flexShrink: 0, justifyContent: "center" },
  start: { textAlign: "left" },
  numeric: { textAlign: "right", fontVariantNumeric: "tabular-nums" },
  error: { color: color.negative },
  detail: { marginInline: space.s1, marginBottom: space.s1_5, display: "grid", gap: space.s2 },
  field: { display: "grid", gap: space.s0_5 },
  attrGrid: {
    display: "grid",
    gridTemplateColumns: "auto minmax(0, 1fr)",
    columnGap: space.s3,
    rowGap: space.s0_5,
  },
  attrValue: { wordBreak: "break-all", color: color.fgMuted },
  // A trace id or an error message is something the reader copies out, so it opts back in to
  // selection that the shell turns off everywhere else.
  selectable: { userSelect: "text" },
});

const STATUS_TONE: Record<SpanRow["status"], Tone> = {
  error: "negative",
  ok: "success",
  unset: "neutral",
};

function StatusTag({ status }: { status: SpanRow["status"] }) {
  return <span {...stylex.props(toneInk[STATUS_TONE[status]])}>{status}</span>;
}

function SpanRowItem({
  span,
  open,
  onToggle,
}: {
  span: SpanRow;
  open: boolean;
  onToggle: () => void;
}) {
  const panelId = useId();
  return (
    <div>
      <Pressable
        type="button"
        onClick={onToggle}
        aria-expanded={open}
        aria-controls={panelId}
        className={stylex.props(tr.spanRow, typeStep.uiMd).className}
      >
        <span {...stylex.props(spanColumns.chevron, tr.chevronBox)}>
          <Icon
            name="chevron-down"
            size="xs"
            className={stylex.props(chevron.base, vocab.faint, !open && chevron.shut).className}
          />
        </span>
        <span {...stylex.props(spanColumns.name, vocab.truncate, tr.start)}>{span.name}</span>
        <span {...stylex.props(spanColumns.duration, vocab.hold, tr.numeric)}>
          {span.durationMillis.toFixed(1)}ms
        </span>
        <span {...stylex.props(spanColumns.status, vocab.hold, tr.start)}>
          <StatusTag status={span.status} />
        </span>
        <span
          {...stylex.props(spanColumns.trace, vocab.hold, vocab.truncate, tr.start, vocab.faint)}
        >
          {span.traceId.slice(0, 12)}
        </span>
      </Pressable>
      {open && (
        <div id={panelId}>
          <SpanDetail span={span} />
        </div>
      )}
    </div>
  );
}

function SpanDetail({ span }: { span: SpanRow }) {
  const meta: [string, string][] = [
    ["trace", span.traceId],
    ["span", span.id],
    ["parent", span.parentSpanId ?? "—"],
    ["kind", span.kind],
    ["start", new Date(span.startMs).toISOString()],
    ["dur", `${span.durationMillis.toFixed(1)}ms`],
  ];
  const attrs = Object.entries(span.attrs);
  return (
    <Well as="div" className={stylex.props(tr.detail).className}>
      {span.statusMessage && (
        <Field label="error">
          <span {...stylex.props(vocab.wrapText, tr.error, tr.selectable)}>
            {span.statusMessage}
          </span>
        </Field>
      )}
      <KeyValues rows={meta} />
      {attrs.length > 0 && (
        <Field label="attributes">
          <KeyValues rows={attrs.map(([k, v]) => [k, String(v)])} />
        </Field>
      )}
    </Well>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div {...stylex.props(tr.field)}>
      <div {...stylex.props(vocab.faint, typeStep.uiXs)}>{label}</div>
      {children}
    </div>
  );
}

function KeyValues({ rows }: { rows: [string, string][] }) {
  return (
    <div {...stylex.props(tr.attrGrid)}>
      {rows.map(([k, v]) => (
        <Fragment key={k}>
          <div {...stylex.props(vocab.faint)}>{k}</div>
          <div {...stylex.props(tr.attrValue, tr.selectable)}>{v}</div>
        </Fragment>
      ))}
    </div>
  );
}

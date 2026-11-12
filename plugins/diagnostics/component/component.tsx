import { useCallback, useEffect, useMemo, useState } from "react";
import {
  failureText,
  type ComponentContext,
  type TelemetrySnapshot,
  type SpanRow,
  type MetricRow,
  type LogRow,
  type Translate,
  useTranslator,
} from "@flame/component-sdk";
import { Activity, Refresh, Gauge, FileText } from "@flame/icons/react";
import {
  Toolbar,
  Panel,
  PanelHeader,
  PanelSection,
  ActionMenu,
  Segmented,
  IconButton,
  CodeBlock,
  Disclosure,
  EmptyState,
  Facts,
  Metadata,
  RecordGroup,
  RecordSummary,
  RecordBody,
  Badge,
  type Fact,
} from "@flame/ui";
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { de } from "./locales/de";

const CATALOGS = { en, zh, "zh-TW": zhTW, ja, ko, es, fr, de };

type Signal = "spans" | "metrics" | "logs";
type Observation = SpanRow | MetricRow | LogRow;

function title(row: Observation, t: Translate, clock: Intl.DateTimeFormat) {
  if ("durationMillis" in row)
    return (
      <RecordSummary
        icon={Activity}
        title={row.name}
        description={t("span.meta", { ms: row.durationMillis, kind: row.kind })}
        status={
          <Badge tone={row.status === "error" ? "negative" : "neutral"}>
            {t(`span.status.${row.status}`)}
          </Badge>
        }
      />
    );
  if ("severity" in row)
    return (
      <RecordSummary
        icon={FileText}
        title={
          row.body
            .split(/\r?\n/)
            .find((line) => line.trim())
            ?.trim() ?? row.severity
        }
        description={
          <time dateTime={new Date(row.timeMs).toISOString()}>
            {clock.format(new Date(row.timeMs))}
          </time>
        }
        status={<Badge>{row.severity}</Badge>}
      />
    );
  return (
    <RecordSummary
      icon={Gauge}
      title={row.name}
      description={row.description || undefined}
      status={<Badge>{t(`metric.kind.${row.kind}`)}</Badge>}
    />
  );
}

function DiagnosticsReport({
  snapshot,
  signal,
  t,
  locale,
}: {
  snapshot: TelemetrySnapshot;
  signal: Signal;
  t: Translate;
  locale: string;
}) {
  const rows = snapshot[signal];
  const clock = useMemo(
    () => new Intl.DateTimeFormat(locale, { timeStyle: "medium" }),
    [locale],
  );
  return (
    <RecordGroup
      variant="list"
      id="output"
      aria-label={t("observations.title")}
    >
      {rows.map((row) => (
        <Disclosure
          variant="row"
          key={`${signal}:${row.id}`}
          title={title(row, t, clock)}
        >
          {"durationMillis" in row && (
            <>
              {row.statusMessage && (
                <RecordBody>{row.statusMessage}</RecordBody>
              )}
              <Facts
                values={[
                  [
                    t("fact.started"),
                    new Date(row.startMs).toLocaleString(locale),
                  ],
                  [
                    t("fact.duration"),
                    t("duration.ms", { ms: row.durationMillis }),
                  ],
                  [t("fact.trace"), row.traceId],
                  [t("fact.parentSpan"), row.parentSpanId],
                ]}
              />
            </>
          )}
          {"severity" in row && (
            <>
              <RecordBody>{row.body}</RecordBody>
              <Facts
                values={[
                  [
                    t("fact.recorded"),
                    new Date(row.timeMs).toLocaleString(locale),
                  ],
                  [t("fact.trace"), row.traceId],
                  [t("fact.span"), row.spanId],
                ]}
              />
            </>
          )}
          {"count" in row && (
            <Facts
              values={(["count", "sum", "p50", "p95", "avg"] as const)
                .filter((key) => row[key] !== undefined)
                .map(
                  (key) =>
                    [
                      t(`metric.${key}`),
                      row.unit && (key !== "count" || row.kind !== "histogram")
                        ? `${row[key]} ${row.unit}`
                        : String(row[key]),
                    ] as Fact,
                )}
            />
          )}
          {Object.keys(row.attrs).length > 0 && (
            <PanelSection title={t("attributes.title")}>
              <Facts
                values={Object.entries(row.attrs).map(([key, value]) => [
                  key,
                  String(value),
                ])}
              />
            </PanelSection>
          )}
          <Disclosure title={t("raw.title")}>
            <CodeBlock
              label={t("raw.label")}
              language="json"
              labels={{
                copy: t("code.copy"),
                copied: t("code.copied"),
                wrap: t("code.wrap"),
                unwrap: t("code.unwrap"),
              }}
            >
              {JSON.stringify(row, null, 2)}
            </CodeBlock>
          </Disclosure>
        </Disclosure>
      ))}
      {!rows.length && (
        <EmptyState
          icon={Activity}
          size="compact"
          title={t("observations.empty.title")}
        />
      )}
    </RecordGroup>
  );
}
type Read = {
  snapshot?: TelemetrySnapshot;
  error?: { operation: "read" | "clear"; cause: unknown };
};

export function Diagnostics({ context }: { context: ComponentContext }) {
  const { telemetry, signal: componentSignal } = context;
  const t = useTranslator(CATALOGS, context.locale);
  const numbers = useMemo(
    () => new Intl.NumberFormat(context.locale),
    [context.locale],
  );
  const [signal, setSignal] = useState<Signal>("spans");
  const [read, setRead] = useState<Read>({});
  const refresh = useCallback(() => {
    try {
      componentSignal.throwIfAborted();
      setRead({ snapshot: telemetry.read() });
    } catch (error) {
      setRead((previous) => ({
        ...previous,
        error: { operation: "read", cause: error },
      }));
    }
  }, [telemetry, componentSignal]);
  useEffect(refresh, [refresh]);
  function clear() {
    try {
      componentSignal.throwIfAborted();
      telemetry.clear();
      setSignal("spans");
      setRead({});
      refresh();
    } catch (error) {
      setRead((previous) => ({
        ...previous,
        error: { operation: "clear", cause: error },
      }));
    }
  }
  const detail =
    read.error &&
    failureText(read.error.cause, t(`${read.error.operation}.failed`));
  return (
    <Panel
      label={t("panel.title")}
      header={<PanelHeader title={t("panel.title")} />}
      controls={
        <Toolbar label={t("controls.label")}>
          <IconButton
            icon={Refresh}
            aria-label={t("refresh")}
            size="md"
            disabled={componentSignal.aborted}
            onClick={refresh}
          />
          <ActionMenu
            label={t("actions.label")}
            disabled={componentSignal.aborted}
            items={[
              {
                label: t("actions.clear"),
                tone: "negative",
                onSelect: clear,
              },
            ]}
          />
          <Segmented
            ariaLabel={t("signal.label")}
            value={signal}
            onChange={(value) => setSignal(value as Signal)}
            options={(["spans", "metrics", "logs"] as const).map((value) => ({
              value,
              label: read.snapshot
                ? t("signal.count", {
                    signal: t(`signal.${value}`),
                    count: numbers.format(read.snapshot[value].length),
                  })
                : t(`signal.${value}`),
            }))}
          />
        </Toolbar>
      }
    >
      {detail && (
        <div role="alert">
          <RecordBody>{detail}</RecordBody>
        </div>
      )}
      {read.snapshot && (
        <>
          <Metadata role="status">
            {t(read.error ? "observations.stale" : "observations.accepted")}
          </Metadata>
          <DiagnosticsReport
            snapshot={read.snapshot}
            signal={signal}
            t={t}
            locale={context.locale}
          />
        </>
      )}
      <Disclosure title={t("about.title")}>
        <Metadata>{t("about.note")}</Metadata>
      </Disclosure>
    </Panel>
  );
}

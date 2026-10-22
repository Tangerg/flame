import { useCallback, useEffect, useMemo, useState } from "react";
import {
  createTranslator,
  type ComponentContext,
  type TelemetrySnapshot,
  type SpanRow,
  type MetricRow,
  type LogRow,
  type Translate,
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
  Stats,
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

function title(row: Observation, t: Translate, locale: string) {
  if ("durationMillis" in row)
    return (
      <RecordSummary
        icon={Activity}
        title={row.name}
        description={t("span.meta", { ms: row.durationMillis, kind: row.kind })}
        status={
          <Badge tone={row.status === "error" ? "negative" : "neutral"}>
            {row.status}
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
            {new Intl.DateTimeFormat(locale, {
              timeStyle: "medium",
            }).format(new Date(row.timeMs))}
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
      status={<Badge>{row.kind}</Badge>}
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
  return (
    <>
      <Stats
        aria-label={t("counts.label")}
        values={[
          [t("signal.spans"), snapshot.spans.length],
          [t("signal.metrics"), snapshot.metrics.length],
          [t("signal.logs"), snapshot.logs.length],
        ]}
      />
      <PanelSection
        title={t("observations.title")}
        description={t("observations.summary", { count: rows.length })}
      >
        <RecordGroup
          variant="list"
          id="output"
          aria-label={t("observations.title")}
        >
          {rows.map((row) => (
            <Disclosure
              variant="row"
              key={`${signal}:${row.id}`}
              title={title(row, t, locale)}
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
                          key,
                          `${row[key]}${row.unit ? ` ${row.unit}` : ""}`,
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
                <CodeBlock label={t("raw.label")} language="json">
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
              sub={t("observations.empty.sub")}
            />
          )}
        </RecordGroup>
      </PanelSection>
    </>
  );
}
type Read = { snapshot?: TelemetrySnapshot; error?: string };

export function Diagnostics({ context }: { context: ComponentContext }) {
  const t = useMemo(
    () => createTranslator(CATALOGS, context.locale),
    [context.locale],
  );
  const [signal, setSignal] = useState<Signal>("spans");
  const [read, setRead] = useState<Read>({});
  const refresh = useCallback(() => {
    try {
      context.signal.throwIfAborted();
      setRead({ snapshot: context.telemetry.read() });
    } catch (error) {
      setRead((previous) => ({
        ...previous,
        error: error instanceof Error ? error.message : t("read.failed"),
      }));
    }
  }, [context, t]);
  useEffect(refresh, [refresh]);
  function clear() {
    try {
      context.signal.throwIfAborted();
      context.telemetry.clear();
      setSignal("spans");
      setRead({});
      refresh();
    } catch (error) {
      setRead((previous) => ({
        ...previous,
        error: error instanceof Error ? error.message : t("clear.failed"),
      }));
    }
  }
  return (
    <Panel
      label={t("panel.title")}
      header={
        <PanelHeader
          title={t("panel.title")}
          description={t("panel.description")}
        />
      }
      controls={
        <Toolbar label={t("controls.label")}>
          <IconButton
            icon={Refresh}
            aria-label={t("refresh")}
            size="md"
            disabled={context.signal.aborted}
            onClick={refresh}
          />
          <ActionMenu
            label={t("actions.label")}
            disabled={context.signal.aborted}
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
            options={[
              { value: "spans", label: t("signal.spans") },
              { value: "metrics", label: t("signal.metrics") },
              { value: "logs", label: t("signal.logs") },
            ]}
          />
        </Toolbar>
      }
    >
      {read.error && <p role="alert">{read.error}</p>}
      {read.snapshot && (
        <DiagnosticsReport
          snapshot={read.snapshot}
          signal={signal}
          t={t}
          locale={context.locale}
        />
      )}
      <Disclosure title={t("about.title")}>
        <Metadata>{t("about.note")}</Metadata>
      </Disclosure>
    </Panel>
  );
}

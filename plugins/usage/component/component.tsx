import {
  type ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  failureText,
  createTranslator,
  type ComponentContext,
  type Translate,
} from "@flame/component-sdk";
import { ChartColumn, Refresh } from "@flame/icons/react";
import type { UsageSummary } from "@flame/runtime-client/wire";
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
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { de } from "./locales/de";

const CATALOGS = { en, zh, "zh-TW": zhTW, ja, ko, es, fr, de };

type Formatters = {
  number: (value: number) => string;
  cost: (amount?: number) => string;
};

function makeFormatters(locale: string, t: Translate): Formatters {
  const number = new Intl.NumberFormat(locale);
  const dollars = new Intl.NumberFormat(locale, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 6,
  });
  return {
    number: (value) => number.format(value),
    cost: (amount) =>
      amount === undefined ? t("cost.notReported") : dollars.format(amount),
  };
}

const FIELDS = [
  "inputTokens",
  "outputTokens",
  "cacheReadTokens",
  "cacheWriteTokens",
  "reasoningTokens",
] as const;

const GROUPS = [
  "byProvider",
  "byModel",
  "byDay",
  "byOrigin",
  "byScope",
] as const;

export function UsageReport({
  report,
  group,
  filter,
  controls,
  t,
  fmt,
}: {
  report: UsageSummary;
  group: string;
  filter: string;
  controls: ReactNode;
  t: Translate;
  fmt: Formatters;
}) {
  const buckets = report[group as (typeof GROUPS)[number]] ?? [];
  const matching = buckets.filter((bucket) =>
    bucket.key.toLocaleLowerCase().includes(filter.toLocaleLowerCase()),
  );
  return (
    <>
      <PanelSection
        title={t("overview.title")}
        description={t("overview.summary", {
          calls: fmt.number(report.coverage.calls),
          reported: fmt.number(report.coverage.reportedCalls),
          priced: fmt.number(report.coverage.pricedCalls),
        })}
      >
        <Stats
          id="totals"
          aria-label={t("totals.label")}
          values={[
            report.total.costUsd === undefined
              ? [t("fact.knownSubtotal"), fmt.cost(report.recordedCostUsd)]
              : [t("fact.totalCost"), fmt.cost(report.total.costUsd)],
            [
              t("fact.known.inputTokens"),
              fmt.number(report.total.inputTokens ?? 0),
            ],
            [
              t("fact.known.outputTokens"),
              fmt.number(report.total.outputTokens ?? 0),
            ],
          ]}
        />
        <Disclosure title={t("accounting.title")}>
          <Facts
            values={[
              [t("fact.knownSubtotal"), fmt.cost(report.recordedCostUsd)],
              [t("fact.totalCost"), fmt.cost(report.total.costUsd)],
              ...FIELDS.map(
                (key) =>
                  [
                    t(`fact.known.${key}`),
                    fmt.number(report.total[key] ?? 0),
                  ] as const,
              ),
              [t("fact.attempts"), fmt.number(report.coverage.calls)],
              [
                t("fact.usageRecorded"),
                `${report.coverage.reportedCalls}/${report.coverage.calls}`,
              ],
              [
                t("fact.pricedAttempts"),
                `${report.coverage.pricedCalls}/${report.coverage.calls}`,
              ],
              [t("fact.inProgress"), fmt.number(report.coverage.startedCalls)],
              [
                t("fact.outcomeUnknown"),
                fmt.number(report.coverage.unknownCalls),
              ],
              [t("fact.sessions"), fmt.number(report.sessions)],
              [t("fact.runs"), fmt.number(report.runs)],
            ]}
          />
          <Metadata>{t("accounting.note")}</Metadata>
        </Disclosure>
      </PanelSection>
      <PanelSection
        id="breakdown"
        aria-label={t("breakdown.title")}
        title={t("breakdown.title")}
      >
        {controls}
        <div id="empty" hidden={report.coverage.calls !== 0}>
          <EmptyState
            icon={ChartColumn}
            size="compact"
            title={t("empty.title")}
          />
        </div>
        {report.coverage.calls !== 0 && (
          <RecordGroup variant="list" aria-label={t("breakdown.group")}>
            {matching.map((bucket) => (
              <ListRow key={bucket.key}>
                <RecordHeading
                  title={bucket.key}
                  actions={
                    <MetricValue>
                      {t("bucket.costUsd", {
                        cost: fmt.cost(bucket.recordedCostUsd),
                      })}
                    </MetricValue>
                  }
                  description={
                    <RecordMeta>
                      <span>
                        {t("bucket.attempts", {
                          calls: fmt.number(bucket.coverage.calls),
                        })}
                      </span>
                      <span>
                        {t("bucket.knownInput", {
                          tokens: fmt.number(bucket.total.inputTokens ?? 0),
                        })}
                      </span>
                      <span>
                        {t("bucket.knownOutput", {
                          tokens: fmt.number(bucket.total.outputTokens ?? 0),
                        })}
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
                <Disclosure title={t("bucket.details")}>
                  <Facts
                    values={[
                      [t("fact.knownCost"), fmt.cost(bucket.recordedCostUsd)],
                      [
                        t("fact.usagePerAttempts"),
                        `${bucket.coverage.reportedCalls}/${bucket.coverage.calls}`,
                      ],
                      [
                        t("fact.pricedPerAttempts"),
                        `${bucket.coverage.pricedCalls}/${bucket.coverage.calls}`,
                      ],
                      ...FIELDS.map(
                        (key) =>
                          [
                            t(`fact.known.${key}`),
                            fmt.number(bucket.total[key] ?? 0),
                          ] as const,
                      ),
                    ]}
                  />
                </Disclosure>
              </ListRow>
            ))}
            {!matching.length && (
              <EmptyState size="compact" title={t("breakdown.empty.title")} />
            )}
          </RecordGroup>
        )}
      </PanelSection>
    </>
  );
}

const PERIODS = [
  { value: "all", query: {} },
  { value: "30", query: { sinceDays: 30 } },
  { value: "7", query: { sinceDays: 7 } },
] as const;
type Period = (typeof PERIODS)[number];
type Read = {
  loading: boolean;
  accepted?: { report: UsageSummary; period: Period };
  error?: string;
};

export function Usage({ context }: { context: ComponentContext }) {
  const t = useMemo(
    () => createTranslator(CATALOGS, context.locale),
    [context.locale],
  );
  const fmt = useMemo(
    () => makeFormatters(context.locale, t),
    [context.locale, t],
  );
  const [group, setGroup] = useState("byProvider");
  const [filter, setFilter] = useState("");
  const [period, setPeriod] = useState<Period>(PERIODS[0]);
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
          error: failureText(error, t("read.failed")),
        }));
    }
  }, [context, period, t]);
  useEffect(() => {
    void refresh();
    return () => reading.current?.abort();
  }, [refresh]);
  return (
    <Panel
      label={t("panel.title")}
      header={<PanelHeader title={t("panel.title")} />}
      controls={
        <>
          <Toolbar
            label={t("scope.label")}
            actions={
              <IconButton
                icon={Refresh}
                aria-label={t("refresh")}
                size="md"
                pending={read.loading}
                onClick={() => void refresh()}
              />
            }
          >
            <Select
              size="lg"
              label={t("period.label")}
              value={period.value}
              options={PERIODS.map(({ value }) => ({
                value,
                label: t(`period.${value}`),
              }))}
              onValueChange={(value) =>
                setPeriod(PERIODS.find((item) => item.value === value)!)
              }
            />
          </Toolbar>
          {read.loading && (
            <Metadata role="status">{t("status.reading")}</Metadata>
          )}
          {read.error && (
            <Metadata role="alert">
              {t("status.error", { error: read.error })}
            </Metadata>
          )}
          {read.accepted &&
            (read.loading || read.error || read.accepted.period !== period) && (
              <Metadata role="status">
                {t("status.stale", {
                  period: t(
                    `period.${read.accepted.period.value}`,
                  ).toLocaleLowerCase(),
                })}
              </Metadata>
            )}
        </>
      }
    >
      {read.accepted && (
        <>
          {!read.loading && !read.error && read.accepted.period === period && (
            <Metadata role="status" className="sr-only">
              {t("status.loaded", {
                period: t(`period.${read.accepted.period.value}`),
              })}
            </Metadata>
          )}
          <UsageReport
            report={read.accepted.report}
            group={group}
            filter={filter}
            controls={
              <FilterBar
                label={t("controls.label")}
                search={
                  <SearchField
                    size="lg"
                    id="filter"
                    aria-label={t("filter.label")}
                    placeholder={t("filter.placeholder")}
                    value={filter}
                    onChange={(event) => setFilter(event.target.value)}
                    onClear={() => setFilter("")}
                    clearLabel={t("filter.clear")}
                  />
                }
              >
                <Select
                  size="lg"
                  label={t("group.label")}
                  value={group}
                  onValueChange={setGroup}
                  options={GROUPS.map((value) => ({
                    value,
                    label: t(`group.${value}`),
                  }))}
                />
              </FilterBar>
            }
            t={t}
            fmt={fmt}
          />
        </>
      )}
    </Panel>
  );
}

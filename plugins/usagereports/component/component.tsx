import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type Ref,
} from "react";
import { ArrowLeft, Refresh } from "@flame/icons/react";
import {
  bindOperations,
  createTranslator,
  mutationSettlementIsUnknown,
  type ComponentContext,
  type Translate,
} from "@flame/component-sdk";
import { scrollPositions, type ScrollPosition } from "@flame/component-sdk";
import type { JSONNumber } from "@flame/runtime-client";
import {
  Button,
  Disclosure,
  EmptyState,
  Facts,
  Icon,
  IconButton,
  ListRow,
  Metadata,
  ModalDialog,
  Panel,
  PanelHeader,
  PanelSection,
  PanelTitle,
  RecordGroup,
  RecordHeading,
  Select,
  Toolbar,
} from "@flame/ui";
import {
  CAPABILITY_JSON,
  type MethodInputs,
  type MethodOutputs,
} from "./methods.generated";
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { de } from "./locales/de";

const CATALOGS = { en, zh, "zh-TW": zhTW, ja, ko, es, fr, de };

const captureMethod = "flame.usage-reports/capture.v1";
const listMethod = "flame.usage-reports/list.v1";
const getMethod = "flame.usage-reports/get.v1";
const deleteMethod = "flame.usage-reports/delete.v1";
type Header = MethodOutputs[typeof listMethod][number];
type Report = MethodOutputs[typeof getMethod];
type Period = MethodInputs[typeof captureMethod]["period"];
type BoundContext = ComponentContext & {
  readonly binding: NonNullable<ComponentContext["binding"]>;
};
type ArchiveRead = { loading: boolean; accepted?: Header[]; error?: string };
type OpenedReport = {
  header: Header;
  loading: boolean;
  accepted?: Report;
  error?: string;
};
type ArchiveReturn = { opener?: HTMLElement; scroll: ScrollPosition[] };
const PERIODS = ["daily", "weekly"] as const satisfies readonly Period[];
type Formatters = {
  count: (value?: JSONNumber) => string;
  amount: (value?: JSONNumber) => string;
};
function makeFormatters(locale: string, t: Translate): Formatters {
  const number = new Intl.NumberFormat(locale);
  const dollars = new Intl.NumberFormat(locale, { maximumFractionDigits: 6 });
  const format = (
    value: JSONNumber | undefined,
    formatter: Intl.NumberFormat,
  ) =>
    value === undefined
      ? t("cost.notReported")
      : typeof value === "object"
        ? value.toString()
        : formatter.format(value);
  return {
    count: (value) => format(value, number),
    amount: (value) => format(value, dollars),
  };
}
const failure = (error: unknown) =>
  error instanceof Error ? error.message : String(error);
const observed = (header: Header, locale: string) =>
  new Date(header.observedAt).toLocaleString(locale);
const lowerBound = (header: Header, t: Translate, locale: string) =>
  header.since
    ? new Date(header.since).toLocaleString(locale)
    : t("evidence.beginning");
const reportTitle = (header: Header, t: Translate, locale: string) =>
  t("report.title", {
    period: t(`period.${header.period}.label`),
    observed: observed(header, locale),
  });

export function UsageReports({ context }: { context: ComponentContext }) {
  const bound = useMemo(() => {
    if (!context.binding)
      throw new Error("Saved usage reports require their declared backend");
    return { ...context, binding: context.binding };
  }, [context]);
  const identity = `${bound.binding.installationId}/${bound.binding.digest}/${bound.binding.contribution}`;
  return <Archive key={identity} context={bound} />;
}

function Archive({ context }: { context: BoundContext }) {
  const locale = context.locale;
  const t = useMemo(() => createTranslator(CATALOGS, locale), [locale]);
  const fmt = useMemo(() => makeFormatters(locale, t), [locale, t]);
  const operations = useMemo(
    () =>
      bindOperations<MethodInputs, MethodOutputs>(context, CAPABILITY_JSON, {
        scope: "installation",
        reference: context.binding.installationId,
      }),
    [context],
  );
  const [archive, setArchive] = useState<ArchiveRead>({ loading: true });
  const [opened, setOpened] = useState<OpenedReport>();
  const [removing, setRemoving] = useState<Header>();
  const [period, setPeriod] = useState<Period>("daily");
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState<string>();
  const lifetime = useRef<AbortController>(undefined);
  const inspecting = useRef<AbortController>(undefined);
  const listing = useRef<AbortController>(undefined);
  const panel = useRef<HTMLElement>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  const refreshButton = useRef<HTMLButtonElement>(null);
  const backButton = useRef<HTMLButtonElement>(null);
  const removeButton = useRef<HTMLButtonElement>(null);
  const cancelRemoval = useRef<HTMLButtonElement>(null);
  // Command outcomes settle after awaits; they decide against the committed
  // view rather than the view captured when the command started.
  const shown = useRef<OpenedReport>(undefined);
  const archiveReturn = useRef<ArchiveReturn>(undefined);
  const pendingFocus = useRef<{
    target: "report" | "archive";
    active: Element | null;
  }>(null);
  const unknown = [
    ...operations.pendingCommands(captureMethod),
    ...operations.pendingCommands(deleteMethod),
  ];

  const refresh = useCallback(
    async (owner: AbortSignal): Promise<Header[] | undefined> => {
      listing.current?.abort();
      const request = new AbortController();
      listing.current = request;
      const signal = AbortSignal.any([owner, request.signal]);
      setArchive((previous) => ({
        ...previous,
        loading: true,
        error: undefined,
      }));
      try {
        const result = await operations.query(listMethod, {}, signal);
        if (signal.aborted) return;
        setArchive({ loading: false, accepted: result });
        return result;
      } catch (error) {
        if (!signal.aborted)
          setArchive((previous) => ({
            ...previous,
            loading: false,
            error: failure(error),
          }));
      }
    },
    [operations],
  );

  useEffect(() => {
    const owner = new AbortController();
    lifetime.current = owner;
    void refresh(AbortSignal.any([owner.signal, context.signal]));
    return () => {
      owner.abort();
      inspecting.current?.abort();
      listing.current?.abort();
    };
  }, [context.signal, refresh]);

  useLayoutEffect(() => {
    shown.current = opened;
  }, [opened]);

  const openedId = opened?.header.id;
  useLayoutEffect(() => {
    const requested = pendingFocus.current;
    pendingFocus.current = null;
    if (!requested) return;
    const active = panel.current?.ownerDocument.activeElement;
    const focusIsUnclaimed =
      active === requested.active || active === active?.ownerDocument.body;
    if (requested.target === "report") {
      for (const position of scrollPositions(panel.current))
        position.element.scrollTop = 0;
      if (focusIsUnclaimed) heading.current?.focus();
      return;
    }
    const restore = archiveReturn.current;
    archiveReturn.current = undefined;
    for (const position of restore?.scroll ?? [])
      position.element.scrollTop = position.top;
    if (focusIsUnclaimed)
      (restore?.opener?.isConnected
        ? restore.opener
        : refreshButton.current
      )?.focus();
  }, [openedId]);

  function ownerSignal() {
    const owner = lifetime.current;
    if (!owner) throw new Error("Report component lifetime has not started");
    return AbortSignal.any([context.signal, owner.signal]);
  }

  function activeElement() {
    return panel.current?.ownerDocument.activeElement ?? null;
  }

  async function read(id: string) {
    inspecting.current?.abort();
    const request = new AbortController();
    inspecting.current = request;
    const signal = AbortSignal.any([ownerSignal(), request.signal]);
    setOpened((previous) =>
      previous?.header.id === id
        ? { ...previous, loading: true, error: undefined }
        : previous,
    );
    try {
      const accepted = await operations.query(getMethod, { id }, signal);
      if (!signal.aborted)
        setOpened((previous) =>
          previous?.header.id === id
            ? { ...previous, loading: false, accepted }
            : previous,
        );
    } catch (error) {
      if (!signal.aborted)
        setOpened((previous) =>
          previous?.header.id === id
            ? { ...previous, loading: false, error: failure(error) }
            : previous,
        );
    }
  }

  function open(
    header: Header,
    opener: HTMLElement | undefined,
    active: Element | null,
  ) {
    if (!shown.current)
      archiveReturn.current = {
        opener,
        scroll: scrollPositions(panel.current),
      };
    pendingFocus.current = { target: "report", active };
    setOpened({ header: structuredClone(header), loading: true });
    void read(header.id);
  }

  function leave(active: Element | null, returnToOpener: boolean) {
    inspecting.current?.abort();
    if (!returnToOpener && archiveReturn.current)
      archiveReturn.current.opener = undefined;
    pendingFocus.current = { target: "archive", active };
    setOpened(undefined);
  }

  async function observe(command: Promise<Header | boolean>, removed?: Header) {
    const signal = ownerSignal();
    const active = activeElement();
    setPending(true);
    setMessage(undefined);
    try {
      const result = await command;
      if (signal.aborted) return;
      if (typeof result !== "boolean") {
        const current = activeElement();
        // A later completion opens the saved report only while the user
        // remains where the command started.
        if (current === active || current === current?.ownerDocument.body) {
          setMessage(t("message.saved"));
          open(result, undefined, active);
        } else
          setMessage(
            t("message.savedArchived", {
              title: reportTitle(result, t, locale),
            }),
          );
        void refresh(signal);
        return;
      }
      setMessage(t(removed ? "message.removed" : "message.removalConfirmed"));
      const accepted = await refresh(signal);
      if (signal.aborted) return;
      // A recovered removal reports success without its original target. Only
      // the captured target or fresh canonical membership can retire the open
      // report; an unread archive keeps it.
      const current = shown.current;
      if (
        current &&
        (current.header.id === removed?.id ||
          (accepted && !accepted.some(({ id }) => id === current.header.id)))
      )
        leave(active, false);
    } catch (error) {
      if (!signal.aborted)
        setMessage(
          mutationSettlementIsUnknown(error)
            ? t("message.unknown")
            : t("message.archiveFailed", { error: failure(error) }),
        );
    } finally {
      if (!signal.aborted) {
        setPending(false);
        setRemoving(undefined);
      }
    }
  }

  function save() {
    if (pending || unknown.length) return;
    try {
      void observe(operations.command(captureMethod, { period }));
    } catch (error) {
      setMessage(t("message.captureRejected", { error: failure(error) }));
    }
  }

  function requestRemoval() {
    if (!opened || pending || unknown.length) return;
    setMessage(undefined);
    setRemoving(structuredClone(opened.header));
  }

  function confirmRemoval() {
    if (!removing || pending || unknown.length) return;
    try {
      void observe(
        operations.command(deleteMethod, { id: removing.id }),
        removing,
      );
    } catch (error) {
      setRemoving(undefined);
      setMessage(t("message.removalRejected", { error: failure(error) }));
    }
  }

  function dismissRemoval() {
    if (!pending) setRemoving(undefined);
  }

  const removalReturn = useCallback(() => {
    for (const element of [removeButton.current, backButton.current])
      if (element?.isConnected && !element.disabled) return element;
    return refreshButton.current;
  }, []);

  async function retire(command: (typeof unknown)[number]) {
    const signal = ownerSignal();
    setPending(true);
    try {
      await command.retire(signal);
      if (!signal.aborted) setMessage(t("message.retired"));
    } catch (error) {
      if (!signal.aborted)
        setMessage(t("message.retireFailed", { error: failure(error) }));
    } finally {
      if (!signal.aborted) setPending(false);
    }
  }

  const headers = archive.accepted;
  return (
    <Panel
      ref={panel}
      label={t("panel.title")}
      header={
        <PanelHeader
          title={t("panel.title")}
          description={opened ? undefined : t("panel.description")}
        />
      }
      controls={
        opened ? (
          <>
            <Toolbar
              label={t("report.scope.label")}
              actions={
                <IconButton
                  icon={Refresh}
                  aria-label={t("report.refresh")}
                  size="md"
                  pending={opened.loading}
                  onClick={() => void read(opened.header.id)}
                />
              }
            >
              <Button
                ref={backButton}
                variant="ghost"
                size="sm"
                onClick={() => leave(activeElement(), true)}
              >
                <Icon glyph={ArrowLeft} size="sm" />
                {t("report.back")}
              </Button>
              <PanelTitle ref={heading} tabIndex={-1} size="sm" aria-level={2}>
                {reportTitle(opened.header, t, locale)}
              </PanelTitle>
            </Toolbar>
          </>
        ) : (
          <Toolbar label={t("archive.actions.label")}>
            <Select
              label={t("capture.period.label")}
              value={period}
              onValueChange={(value) => setPeriod(value as Period)}
              options={PERIODS.map((value) => ({
                value,
                label: t("period.windowed", {
                  label: t(`period.${value}.label`),
                  window: t(`period.${value}.window`),
                }),
              }))}
            />
            <Button disabled={pending || unknown.length > 0} onClick={save}>
              {t("capture.save")}
            </Button>
            <IconButton
              ref={refreshButton}
              icon={Refresh}
              aria-label={t("archive.refresh")}
              size="md"
              pending={archive.loading}
              onClick={() => void refresh(ownerSignal())}
            />
          </Toolbar>
        )
      }
    >
      {message && <Metadata role="status">{message}</Metadata>}
      {unknown.map((command) => (
        <Toolbar key={command.idempotencyKey} label={t("pending.label")}>
          <Metadata>{t("pending.note")}</Metadata>
          <Button
            disabled={pending}
            onClick={() => void observe(command.recover(ownerSignal()))}
          >
            {t("pending.check")}
          </Button>
          <Button disabled={pending} onClick={() => void retire(command)}>
            {t("pending.retire")}
          </Button>
        </Toolbar>
      ))}
      {opened && (
        <SavedReport
          opened={opened}
          removeButton={removeButton}
          pending={pending || unknown.length > 0}
          remove={requestRemoval}
          t={t}
          locale={locale}
          fmt={fmt}
        />
      )}
      <div hidden={Boolean(opened)}>
        <PanelSection
          title={t("archive.title")}
          description={headers && t("archive.count", { count: headers.length })}
        >
          <Metadata>
            {t("archive.capture.note", {
              daily: t("period.daily.window"),
              weekly: t("period.weekly.window"),
            })}
          </Metadata>
          {archive.loading && !headers && (
            <Metadata role="status">{t("archive.reading")}</Metadata>
          )}
          {archive.error && (
            <Metadata role="alert">
              {t(headers ? "archive.error.retained" : "archive.error", {
                error: archive.error,
              })}
            </Metadata>
          )}
          {headers && !headers.length && (
            <EmptyState
              title={t("archive.empty.title")}
              sub={t("archive.empty.sub")}
            />
          )}
          {headers && headers.length > 0 && (
            <RecordGroup variant="list" aria-label={t("archive.list.label")}>
              {headers.map((header) => (
                <ListRow key={header.id}>
                  <RecordHeading
                    title={t("row.title", {
                      period: t(`period.${header.period}.label`),
                      observed: observed(header, locale),
                    })}
                    description={t("row.description", {
                      window: t(`period.${header.period}.window`),
                      since: lowerBound(header, t, locale),
                    })}
                    actions={
                      <Button
                        aria-label={t("report.view.aria", {
                          title: reportTitle(header, t, locale),
                        })}
                        onClick={(event) => {
                          setMessage(undefined);
                          open(header, event.currentTarget, activeElement());
                        }}
                      >
                        {t("report.view")}
                      </Button>
                    }
                  />
                </ListRow>
              ))}
            </RecordGroup>
          )}
        </PanelSection>
      </div>
      {removing && (
        <ModalDialog
          open
          role="alertdialog"
          title={t("removal.title", {
            title: reportTitle(removing, t, locale),
          })}
          description={t("removal.description")}
          closeLabel={t("removal.close")}
          size="sm"
          initialFocus={cancelRemoval}
          finalFocus={removalReturn}
          dismissible={!pending}
          onOpenChange={(open) => {
            if (!open) dismissRemoval();
          }}
          footer={
            <>
              <Button
                ref={cancelRemoval}
                variant="ghost"
                disabled={pending}
                onClick={dismissRemoval}
              >
                {t("removal.cancel")}
              </Button>
              <Button
                variant="tonal"
                tone="negative"
                pending={pending}
                aria-busy={pending}
                onClick={confirmRemoval}
              >
                {pending ? t("removal.removing") : t("report.remove")}
              </Button>
            </>
          }
        >
          <Facts
            values={[
              [
                t("fact.period"),
                t("period.windowed", {
                  label: t(`period.${removing.period}.label`),
                  window: t(`period.${removing.period}.window`),
                }),
              ],
              [t("fact.observation"), observed(removing, locale)],
              [t("fact.lowerBound"), lowerBound(removing, t, locale)],
            ]}
          />
        </ModalDialog>
      )}
    </Panel>
  );
}

function SavedReport({
  opened,
  removeButton,
  pending,
  remove,
  t,
  locale,
  fmt,
}: {
  opened: OpenedReport;
  removeButton: Ref<HTMLButtonElement>;
  pending: boolean;
  remove: () => void;
  t: Translate;
  locale: string;
  fmt: Formatters;
}) {
  const { header, accepted: report } = opened;
  const summary = report?.summary;
  return (
    <>
      <Metadata>{t("report.saved.note")}</Metadata>
      <Facts
        values={[
          [
            t("fact.period"),
            t("period.windowed", {
              label: t(`period.${header.period}.label`),
              window: t(`period.${header.period}.window`),
            }),
          ],
          [t("fact.observation"), observed(header, locale)],
          [t("fact.lowerBound"), lowerBound(header, t, locale)],
        ]}
      />
      {opened.loading && (
        <Metadata role="status">{t("report.reading")}</Metadata>
      )}
      {opened.error && (
        <Metadata role="alert">
          {report
            ? t("report.error.refresh", { error: opened.error })
            : t("report.error.read", { error: opened.error })}
        </Metadata>
      )}
      {summary && (
        <PanelSection
          title={t("report.totals.title")}
          description={t("report.totals.description")}
        >
          <Facts
            values={[
              [t("fact.attempts"), fmt.count(summary.coverage.calls)],
              [t("fact.inputTokens"), fmt.count(summary.total.inputTokens)],
              [t("fact.outputTokens"), fmt.count(summary.total.outputTokens)],
              [t("fact.knownSubtotal"), fmt.amount(summary.recordedCostUsd)],
              [t("fact.totalCost"), fmt.amount(summary.total.costUsd)],
              [
                t("fact.usageRecorded"),
                `${summary.coverage.reportedCalls}/${summary.coverage.calls}`,
              ],
              [
                t("fact.pricedAttempts"),
                `${summary.coverage.pricedCalls}/${summary.coverage.calls}`,
              ],
              [
                t("fact.outcomeUnknown"),
                fmt.count(summary.coverage.unknownCalls),
              ],
            ]}
          />
          <Disclosure title={t("report.breakdown.title")}>
            <RecordGroup variant="list">
              {summary.byDay.map((day) => (
                <ListRow key={day.key}>
                  <RecordHeading
                    title={day.key}
                    description={t("report.breakdown.row", {
                      attempts: fmt.count(day.coverage.calls),
                      cost: fmt.amount(day.recordedCostUsd),
                    })}
                  />
                </ListRow>
              ))}
            </RecordGroup>
          </Disclosure>
        </PanelSection>
      )}
      <Disclosure title={t("report.identity.title")}>
        <Facts
          values={[
            [t("fact.reportId"), header.id],
            [t("fact.observedAt"), header.observedAt],
            [t("fact.lowerBoundUtc"), header.since],
          ]}
        />
      </Disclosure>
      <Toolbar label={t("report.actions.label")}>
        <Button
          ref={removeButton}
          variant="tonal"
          tone="negative"
          disabled={pending}
          onClick={remove}
        >
          {t("report.remove")}
        </Button>
      </Toolbar>
    </>
  );
}

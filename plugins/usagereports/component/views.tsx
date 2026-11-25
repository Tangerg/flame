import { type Ref } from "react";
import { Refresh } from "@flame/icons/react";
import { type Translate } from "@flame/component-sdk";
import {
  BackButton,
  Button,
  Disclosure,
  EmptyState,
  Facts,
  IconButton,
  ListRow,
  Metadata,
  ModalDialog,
  PanelSection,
  PanelTitle,
  RecordButton,
  RecordGroup,
  RecordHeading,
  RecordSummary,
  Hang,
  Select,
  Toolbar,
} from "@flame/ui";
import { LIMITS } from "../contracts/limits";
import {
  PERIODS,
  lowerBound,
  observed,
  reportTitle,
  type ArchiveRead,
  type Formatters,
  type Header,
  type OpenedReport,
  type Period,
} from "./model";
import { type ReportArchive } from "./useReportArchive";

export function ArchiveControls({
  archive: { t, fmt, busy, action, refs, catalog, report, recovery },
}: {
  archive: ReportArchive;
}) {
  const opened = report.opened;
  const archive = catalog.read;
  const period = catalog.period;
  const unknown = recovery.commands;
  const { heading, refreshButton, saveButton, backButton } = refs;
  return opened ? (
    <>
      <Toolbar
        label={t("report.scope.label")}
        actions={
          <IconButton
            icon={Refresh}
            aria-label={t("report.refresh")}
            size="md"
            pending={opened.loading}
            onClick={() => report.refresh(opened.header.id)}
          />
        }
      >
        <BackButton
          ref={backButton}
          aria-label={t("report.back")}
          onClick={report.back}
        />
        <PanelTitle ref={heading} tabIndex={-1} size="sm" aria-level={2}>
          {reportTitle(opened.header, t, fmt)}
        </PanelTitle>
      </Toolbar>
    </>
  ) : (
    <Toolbar label={t("archive.actions.label")}>
      <Hang>
        <Select
          label={t("capture.period.label")}
          value={period}
          onValueChange={(value) => catalog.choosePeriod(value as Period)}
          options={PERIODS.map((value) => ({
            value,
            label: t("period.windowed", {
              label: t(`period.${value}.label`),
              window: t(`period.${value}.window`),
            }),
          }))}
        />
      </Hang>
      <Button
        ref={saveButton}
        disabled={busy || unknown.length > 0}
        focusableWhenDisabled
        pending={action?.kind === "capture"}
        aria-busy={action?.kind === "capture"}
        onClick={catalog.save}
      >
        {action?.kind === "capture" ? t("capture.saving") : t("capture.save")}
      </Button>
      <IconButton
        ref={refreshButton}
        icon={Refresh}
        aria-label={t("archive.refresh")}
        size="md"
        pending={archive.loading}
        onClick={catalog.refresh}
      />
    </Toolbar>
  );
}

export function PendingCommands({
  archive: { t, busy, action, recovery },
}: {
  archive: ReportArchive;
}) {
  return recovery.commands.map((command) => {
    const key = command.idempotencyKey;
    const checking = action?.kind === "check" && action.key === key;
    const retiring = action?.kind === "retire" && action.key === key;
    return (
      <Toolbar key={key} label={t("pending.label")}>
        <Metadata>{t("pending.note")}</Metadata>
        <Button
          disabled={busy}
          pending={checking}
          aria-busy={checking}
          onClick={() => recovery.check(command)}
        >
          {checking ? t("pending.checking") : t("pending.check")}
        </Button>
        <Button
          disabled={busy}
          pending={retiring}
          aria-busy={retiring}
          onClick={() => recovery.retire(command)}
        >
          {retiring ? t("pending.retiring") : t("pending.retire")}
        </Button>
      </Toolbar>
    );
  });
}

export function RemovalDialog({
  archive: { t, fmt, busy, action, refs, removal },
}: {
  archive: ReportArchive;
}) {
  const removing = removal.target;
  if (!removing) return null;
  return (
    <ModalDialog
      open
      role="alertdialog"
      title={t("removal.title", {
        title: reportTitle(removing, t, fmt),
      })}
      description={t("removal.description")}
      closeLabel={t("removal.close")}
      size="sm"
      initialFocus={refs.cancelRemoval}
      finalFocus={removal.focusReturn}
      dismissible={!busy}
      onOpenChange={(open) => {
        if (!open) removal.dismiss();
      }}
      footer={
        <>
          <Button
            ref={refs.cancelRemoval}
            variant="ghost"
            disabled={busy}
            onClick={removal.dismiss}
          >
            {t("removal.cancel")}
          </Button>
          <Button
            variant="tonal"
            tone="negative"
            pending={action?.kind === "removal"}
            aria-busy={action?.kind === "removal"}
            onClick={removal.confirm}
          >
            {action?.kind === "removal"
              ? t("removal.removing")
              : t("report.remove")}
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
          [t("fact.observation"), observed(removing, fmt)],
          [t("fact.lowerBound"), lowerBound(removing, t, fmt)],
        ]}
      />
    </ModalDialog>
  );
}

export function SavedReport({
  opened,
  removeButton,
  pending,
  remove,
  t,
  fmt,
}: {
  opened: OpenedReport;
  removeButton: Ref<HTMLButtonElement>;
  pending: boolean;
  remove: () => void;
  t: Translate;
  fmt: Formatters;
}) {
  const { header, accepted: report } = opened;
  const summary = report?.summary;
  return (
    <>
      <Facts
        values={[
          [
            t("fact.period"),
            t("period.windowed", {
              label: t(`period.${header.period}.label`),
              window: t(`period.${header.period}.window`),
            }),
          ],
          [t("fact.observation"), observed(header, fmt)],
          [t("fact.lowerBound"), lowerBound(header, t, fmt)],
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
        <PanelSection title={t("report.totals.title")}>
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
          {summary.byDay.length > 0 && (
            <Disclosure title={t("report.breakdown.title")}>
              <RecordGroup variant="list">
                {summary.byDay.map((day) => (
                  <ListRow key={day.key}>
                    <RecordHeading
                      title={day.key}
                      description={t("report.breakdown.row", {
                        // Selects the plural form only; the exact JSONNumber is displayed.
                        count: Number(day.coverage.calls.toString()),
                        attempts: fmt.count(day.coverage.calls),
                        cost: fmt.amount(day.recordedCostUsd),
                      })}
                    />
                  </ListRow>
                ))}
              </RecordGroup>
            </Disclosure>
          )}
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

export function ArchiveCatalog({
  archive,
  t,
  fmt,
  onOpen,
}: {
  archive: ArchiveRead;
  t: Translate;
  fmt: Formatters;
  onOpen(header: Header, opener: HTMLElement): void;
}) {
  const headers = archive.accepted;
  return (
    <PanelSection
      title={t("archive.title")}
      description={
        headers &&
        t("archive.count", {
          count: headers.length,
          capacity: LIMITS.maxReports,
        })
      }
    >
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
        <EmptyState title={t("archive.empty.title")} />
      )}
      {headers && headers.length > 0 && (
        <RecordGroup variant="list" aria-label={t("archive.list.label")}>
          {headers.map((header) => (
            <RecordButton
              key={header.id}
              onClick={(event) => onOpen(header, event.currentTarget)}
            >
              <RecordSummary
                title={t("row.title", {
                  period: t(`period.${header.period}.label`),
                  observed: observed(header, fmt),
                })}
                description={t("row.description", {
                  since: lowerBound(header, t, fmt),
                })}
              />
            </RecordButton>
          ))}
        </RecordGroup>
      )}
    </PanelSection>
  );
}

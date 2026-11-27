import {
  defineComponent,
  type PluginComponentProps,
} from "@flame/component-sdk";
import {
  Panel,
  PanelHeader,
  PanelSection,
  RecordBody,
  RecordGroup,
  RecordMeta,
} from "@flame/ui";
import {
  useSchedules,
  enabled,
  type ScheduleState,
  UNFILTERED,
} from "./useSchedules";
import { ScheduleDeletion, ScheduleFeedback, ScheduleRow } from "./rows";
import {
  ScheduleCatalogEmpty,
  ScheduleControls,
  SchedulePager,
} from "./catalog";
import { ScheduleEditor } from "./editor";
import { ScheduleCommandObservation } from "./scheduleCommands";

function ScheduleReadStatus({
  read,
  liveError,
  t,
}: Pick<ScheduleState, "read" | "liveError" | "t">) {
  return (
    <>
      {read.loading && <p role="status">{t("reading")}</p>}
      {read.error && (
        <div role="alert">
          <p>{read.accepted ? t("read.errorRetained") : t("read.error")}</p>
          <RecordBody>{read.error}</RecordBody>
        </div>
      )}
      {liveError && (
        <div role="alert">
          <p>{t("live.error")}</p>
          <RecordBody>{liveError}</RecordBody>
        </div>
      )}
    </>
  );
}

// The page number appears only once the catalog spans more than one page.
function CatalogSummary({
  summary,
  page,
  rows,
  t,
}: Pick<ScheduleState, "summary" | "rows" | "t"> & {
  page: number | undefined;
}) {
  return (
    <span ref={summary} tabIndex={-1}>
      <RecordMeta>
        {page !== undefined && <span>{t("pages.current", { page })}</span>}
        <span>
          {t("catalog.enabled", { count: rows.filter(enabled).length })}
        </span>
        <span>
          {t("catalog.disabled", {
            count: rows.filter((item) => !enabled(item)).length,
          })}
        </span>
      </RecordMeta>
    </span>
  );
}

export const Schedules = defineComponent((source) => {
  const observation = new ScheduleCommandObservation(source.signal);
  return (props) => <SchedulesPanel {...props} observation={observation} />;
});

function SchedulesPanel({
  context,
  locale,
  observation,
}: PluginComponentProps & { observation: ScheduleCommandObservation }) {
  const schedules = useSchedules(context, locale, observation);
  const {
    t,
    notice,
    feedbackElement,
    command,
    pending,
    reconcile,
    searchElement,
    filterPage,
    editorOpen,
    read,
    busy,
    openPage,
    summary,
    liveError,
    edit,
    deleting,
    page,
    rows,
    matching,
  } = schedules;
  const cursors = read.accepted?.cursors ?? [];
  const paginated = Boolean(cursors.length) || Boolean(page?.nextCursor);
  const feedback = (notice || command || pending) && (
    <ScheduleFeedback
      t={t}
      notice={notice}
      feedbackElement={feedbackElement}
      command={command}
      pending={pending}
      reconcile={reconcile}
      openSession={context.openSession}
      signal={context.signal}
    />
  );

  return (
    <Panel
      label={t("panel.title")}
      header={<PanelHeader title={t("panel.title")} />}
      controls={<ScheduleControls {...schedules} />}
    >
      <ScheduleReadStatus read={read} liveError={liveError} t={t} />
      {!editorOpen && feedback}
      {edit && (
        <ScheduleEditor {...schedules} edit={edit} feedback={feedback} />
      )}
      {deleting && (
        <ScheduleDeletion
          {...schedules}
          item={deleting.item}
          feedback={feedback}
        />
      )}
      <PanelSection
        title={t("catalog.title")}
        description={
          page && (
            <CatalogSummary
              summary={summary}
              page={paginated ? cursors.length + 1 : undefined}
              rows={rows}
              t={t}
            />
          )
        }
      >
        <RecordGroup
          variant="list"
          id="items"
          aria-label={t("catalog.title")}
          hidden={!page}
        >
          {page && !matching.length && (
            <ScheduleCatalogEmpty
              t={t}
              filtered={rows.length > 0}
              paginated={paginated}
              onClearFilters={() => {
                filterPage("", UNFILTERED);
                searchElement.current?.focus();
              }}
            />
          )}
          {matching.map((item) => (
            <ScheduleRow key={item.id} {...schedules} item={item} />
          ))}
        </RecordGroup>
        {paginated && (
          <SchedulePager
            t={t}
            cursors={cursors}
            nextCursor={page?.nextCursor}
            loading={read.loading}
            busy={busy}
            onPage={openPage}
          />
        )}
      </PanelSection>
    </Panel>
  );
}

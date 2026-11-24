import { type ComponentContext } from "@flame/component-sdk";
import {
  Panel,
  PanelHeader,
  PanelSection,
  RecordBody,
  RecordGroup,
} from "@flame/ui";
import { useSchedules, enabled } from "./useSchedules";
import { ScheduleDeletion, ScheduleFeedback, ScheduleRow } from "./rows";
import {
  ScheduleCatalogEmpty,
  ScheduleControls,
  SchedulePager,
} from "./catalog";
import { ScheduleEditor } from "./editor";

export function Schedules({ context }: { context: ComponentContext }) {
  const openSession = context.openSession;
  const schedules = useSchedules(context);
  const {
    t,
    notice,
    feedbackElement,
    command,
    reconcile,
    searchElement,
    filterPage,
    editorOpen,
    read,
    busy,
    refresh,
    summary,
    liveError,
    edit,
    deleting,
    page,
    rows,
    matching,
  } = schedules;
  const history = read.accepted?.history ?? [];
  const paginated = Boolean(history.length) || Boolean(page?.nextCursor);
  const feedback = (notice || command) && (
    <ScheduleFeedback
      t={t}
      notice={notice}
      feedbackElement={feedbackElement}
      command={command}
      reconcile={reconcile}
      openSession={openSession}
    />
  );

  return (
    <Panel
      label={t("panel.title")}
      header={<PanelHeader title={t("panel.title")} />}
      controls={<ScheduleControls {...schedules} />}
    >
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
            <span ref={summary} tabIndex={-1}>
              {paginated && (
                <>
                  {t("pages.current", { page: history.length + 1 })}
                  {" · "}
                </>
              )}
              {t("catalog.enabled", { count: rows.filter(enabled).length })}
              {" · "}
              {t("catalog.disabled", {
                count: rows.filter((item) => !enabled(item)).length,
              })}
            </span>
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
                filterPage("", "all");
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
            history={history}
            nextCursor={page?.nextCursor}
            loading={read.loading}
            busy={busy}
            onPage={(cursors, control) => void refresh(cursors, control)}
          />
        )}
      </PanelSection>
    </Panel>
  );
}

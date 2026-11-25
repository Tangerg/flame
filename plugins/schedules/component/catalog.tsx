import { CalendarClock, Refresh, Plus, ChevronDown } from "@flame/icons/react";
import {
  Button,
  EmptyState,
  PanelFooter,
  ActionMenu,
  IconButton,
  Icon,
  SearchField,
  FilterBar,
  Select,
  Toolbar,
} from "@flame/ui";
import { templates } from "./templates";
import {
  type ScheduleState,
  type Translate,
  SCHEDULE_FILTERS,
} from "./useSchedules";

export function ScheduleControls({
  t,
  searchElement,
  search,
  filterPage,
  status,
  editorOpen,
  actionsDisabled,
  add,
  read,
  busy,
  refresh,
}: Pick<
  ScheduleState,
  | "t"
  | "searchElement"
  | "search"
  | "filterPage"
  | "status"
  | "editorOpen"
  | "actionsDisabled"
  | "add"
  | "read"
  | "busy"
  | "refresh"
>) {
  return (
    <FilterBar
      label={t("controls.label")}
      search={
        <SearchField
          ref={searchElement}
          size="lg"
          aria-label={t("search.label")}
          placeholder={t("search.placeholder")}
          value={search}
          onChange={(event) => filterPage(event.target.value)}
          onClear={() => filterPage("")}
          clearLabel={t("search.clear")}
        />
      }
      actions={
        <Toolbar label={t("actions.label")}>
          <Button
            variant={editorOpen ? "outline" : "primary"}
            size="lg"
            disabled={actionsDisabled}
            onClick={(event) => add(event.currentTarget)}
          >
            <Icon glyph={Plus} size="sm" /> {t("add")}
          </Button>
          <ActionMenu
            label={t("templates.label")}
            disabled={actionsDisabled}
            triggerContent={
              <>
                {t("templates.trigger")} <Icon glyph={ChevronDown} size="sm" />
              </>
            }
            items={templates.map((template) => ({
              label: t(`template.${template.id}.title`),
              onSelect: (opener) => add(opener, template),
            }))}
          />
        </Toolbar>
      }
    >
      <Select
        size="lg"
        label={t("status.label")}
        value={status}
        onValueChange={(value) => filterPage(search, value)}
        options={SCHEDULE_FILTERS.map((value) => ({
          value,
          label: t(`status.${value}`),
        }))}
      />
      <IconButton
        icon={Refresh}
        aria-label={t("refresh")}
        size="md"
        pending={read.loading}
        disabled={busy}
        onClick={() => void refresh()}
      />
    </FilterBar>
  );
}

// An empty page says why: the filters hid its rows, a later page ran out, or
// nothing is scheduled yet.
export function ScheduleCatalogEmpty({
  t,
  filtered,
  paginated,
  onClearFilters,
}: {
  t: Translate;
  filtered: boolean;
  paginated: boolean;
  onClearFilters(): void;
}) {
  if (filtered)
    return (
      <EmptyState
        icon={CalendarClock}
        size="compact"
        title={t("empty.filtered.title")}
        action={
          <Button variant="outline" onClick={onClearFilters}>
            {t("empty.filtered.action")}
          </Button>
        }
      />
    );
  if (paginated)
    return (
      <EmptyState
        icon={CalendarClock}
        size="compact"
        title={t("empty.page.title")}
      />
    );
  return (
    <EmptyState
      icon={CalendarClock}
      size="compact"
      title={t("empty.none.title")}
    />
  );
}

export function SchedulePager({
  t,
  history,
  nextCursor,
  loading,
  busy,
  onPage,
}: {
  t: Translate;
  history: string[];
  nextCursor: string | undefined;
  loading: boolean;
  busy: boolean;
  onPage(cursors: string[], control: HTMLElement): void;
}) {
  return (
    <PanelFooter>
      <Toolbar label={t("pages.label")}>
        <Button
          pending={loading && !busy}
          focusableWhenDisabled={!busy}
          disabled={busy || !history.length}
          onClick={(event) => onPage(history.slice(0, -1), event.currentTarget)}
        >
          {t("pages.previous")}
        </Button>
        <Button
          pending={loading && !busy}
          focusableWhenDisabled={!busy}
          disabled={busy || !nextCursor}
          onClick={(event) =>
            onPage([...history, nextCursor!], event.currentTarget)
          }
        >
          {t("pages.next")}
        </Button>
      </Toolbar>
    </PanelFooter>
  );
}

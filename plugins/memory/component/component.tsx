import { type ReactNode } from "react";
import { type ComponentContext } from "@flame/component-sdk";
import { Refresh, Plus } from "@flame/icons/react";
import {
  Button,
  Panel,
  PanelHeader,
  IconButton,
  Icon,
  Hang,
  Segmented,
  PanelFooter,
  Disclosure,
  FormField,
  FormActions,
  RecordForm,
  RecordTitle,
  TextArea,
  Checkbox,
  Metadata,
  RecordGroup,
  RecordMeta,
  SearchField,
  FilterBar,
  Select,
  Toolbar,
  isImeKey,
} from "@flame/ui";
import { type Edit, type Scope } from "./types";
import { MemoryDeleteConfirmation } from "./MemoryDeleteConfirmation";
import { MemoryReport } from "./MemoryReport";
import { MemoryFeedback } from "./MemoryFeedback";
import { useMemory } from "./useMemory";
// Installed component bundles use host UI styles without StyleX compilation.
const EDITOR_STYLE = { maxHeight: "40dvh" };
type MemoryState = ReturnType<typeof useMemory>;

export function Memory({ context }: { context: ComponentContext }) {
  const memory = useMemory(context);
  const {
    t,
    edit,
    locked,
    closeEditor,
    editorElement,
    scope,
    cwd,
    project,
    read,
    pending,
    showPage,
    addItem,
    editItem,
    removeItem,
    deleteEdited,
    setSearch,
    clearFilters,
    changeScope,
    searchElement,
    search,
    filtersOpen,
    setFiltersOpen,
    activeFilters,
    status,
    setStatus,
    origin,
    setOrigin,
    pinned,
    setPinned,
    editorFeedback,
    unknown,
    reconcile,
    change,
    current,
    summary,
    recordFeedback,
    recordResult,
    canRestoreFocus,
  } = memory;
  // Outside a Project, Memory's project scope keys on the Session's folder.
  const scopeLabel = (value: Scope) =>
    value === "project" && !context.project ? "folder" : value;
  const recordCommand = unknown?.source === "record" ? unknown : undefined;
  const recovering = unknown?.source === "editor";
  const noTarget = !project && cwd === undefined;
  const editFeedback = (editorFeedback || recovering) && (
    <MemoryFeedback
      canRestoreFocus={canRestoreFocus}
      feedback={editorFeedback}
      original={recovering ? unknown : undefined}
      pending={pending}
      reconcile={reconcile}
      scopeLabel={scopeLabel}
      t={t}
    />
  );
  function clearFiltersAndFocusSearch() {
    clearFilters();
    searchElement.current?.focus();
  }
  return (
    <Panel
      label={t("panel.title")}
      onKeyDown={(event) => {
        if (
          event.key !== "Escape" ||
          event.defaultPrevented ||
          isImeKey(event.nativeEvent) ||
          !edit
        )
          return;
        // This non-modal task keeps independent catalog actions available.
        event.preventDefault();
        event.stopPropagation();
        if (!locked) closeEditor();
      }}
      header={
        <PanelHeader
          size="md"
          title={t("panel.title")}
          context={
            scope === "project" &&
            !project &&
            !cwd && <Metadata>{t("scope.none")}</Metadata>
          }
        />
      }
      controls={
        <>
          <Toolbar
            label={t("nav.label")}
            actions={
              <>
                <IconButton
                  icon={Refresh}
                  aria-label={t("refresh")}
                  size="md"
                  pending={read.loading}
                  disabled={noTarget || pending}
                  onClick={() => showPage(current?.cursors)}
                />
                <Button
                  variant={edit ? "outline" : "primary"}
                  size="md"
                  disabled={locked || noTarget || Boolean(edit)}
                  focusableWhenDisabled
                  onClick={(event) => addItem(event.currentTarget)}
                >
                  <Icon glyph={Plus} size="sm" /> {t("add")}
                </Button>
              </>
            }
          >
            <Hang>
              <Segmented
                ariaLabel={t("segment.label")}
                value={scope}
                options={[
                  {
                    value: "project",
                    label: t(`segment.${scopeLabel("project")}`),
                  },
                  { value: "user", label: t("segment.user") },
                ]}
                onChange={(value) => changeScope(value as Scope)}
                disabled={locked || Boolean(edit)}
              />
            </Hang>
          </Toolbar>
          <MemoryFilters
            t={t}
            searchElement={searchElement}
            search={search}
            setSearch={setSearch}
            filtersOpen={filtersOpen}
            setFiltersOpen={setFiltersOpen}
            activeFilters={activeFilters}
            onClearFilters={clearFiltersAndFocusSearch}
            status={status}
            setStatus={setStatus}
            origin={origin}
            setOrigin={setOrigin}
            pinned={pinned}
            setPinned={setPinned}
          />
        </>
      }
    >
      {read.loading && <p role="status">{t("reading")}</p>}
      {read.error && <p role="alert">{read.error}</p>}
      {edit?.type === "delete" ? (
        <MemoryDeleteConfirmation
          ref={editorElement}
          item={edit.item}
          feedback={editFeedback}
          pending={pending}
          locked={locked}
          recovering={recovering}
          aborted={context.signal.aborted}
          cancel={() => closeEditor()}
          confirm={recovering ? reconcile : deleteEdited}
          t={t}
        />
      ) : edit ? (
        <MemoryEditor
          {...memory}
          edit={edit}
          feedback={editFeedback}
          recovering={recovering}
          aborted={context.signal.aborted}
          scopeLabel={scopeLabel}
        />
      ) : null}
      {(current || recordFeedback || recordCommand) && (
        <>
          <MemoryReport
            canRestoreFocus={canRestoreFocus}
            page={current?.page}
            pageNumber={current?.cursors.length}
            summary={summary}
            search={search}
            status={status}
            origin={origin}
            pinned={pinned}
            clearSearchAndFilters={() => {
              setSearch("");
              clearFiltersAndFocusSearch();
            }}
            feedback={
              <MemoryFeedback
                canRestoreFocus={canRestoreFocus}
                feedback={recordFeedback}
                original={recordCommand}
                pending={pending}
                reconcile={reconcile}
                result={recordResult}
                scopeLabel={scopeLabel}
                t={t}
              />
            }
            resultFocus={recordResult}
            authoring={Boolean(edit)}
            disabled={locked || read.loading || Boolean(read.error)}
            edit={editItem}
            remove={removeItem}
            command={(method, input, intent) =>
              change(method, input, intent, "record")
            }
            t={t}
          />
          {current && (
            <MemoryPager
              t={t}
              cursors={current.cursors}
              nextCursor={current.page.nextCursor}
              loading={read.loading}
              pending={pending}
              onPage={showPage}
            />
          )}
        </>
      )}
    </Panel>
  );
}

function MemoryPager({
  t,
  cursors,
  nextCursor,
  loading,
  pending,
  onPage,
}: {
  t: MemoryState["t"];
  cursors: string[];
  nextCursor: string;
  loading: boolean;
  pending: boolean;
  onPage(cursors: string[], control: HTMLElement): void;
}) {
  if (cursors.length <= 1 && !nextCursor) return null;
  return (
    <PanelFooter>
      <Toolbar label={t("pages.label")}>
        <Button
          pending={loading}
          focusableWhenDisabled
          disabled={pending || cursors.length === 1}
          onClick={(event) => onPage(cursors.slice(0, -1), event.currentTarget)}
        >
          {t("pages.previous")}
        </Button>
        <Button
          pending={loading}
          focusableWhenDisabled
          disabled={pending || !nextCursor}
          onClick={(event) =>
            onPage([...cursors, nextCursor], event.currentTarget)
          }
        >
          {t("pages.next")}
        </Button>
      </Toolbar>
    </PanelFooter>
  );
}

function MemoryFilters({
  t,
  searchElement,
  search,
  setSearch,
  filtersOpen,
  setFiltersOpen,
  activeFilters,
  onClearFilters,
  status,
  setStatus,
  origin,
  setOrigin,
  pinned,
  setPinned,
}: Pick<
  MemoryState,
  | "t"
  | "searchElement"
  | "search"
  | "setSearch"
  | "filtersOpen"
  | "setFiltersOpen"
  | "activeFilters"
  | "status"
  | "setStatus"
  | "origin"
  | "setOrigin"
  | "pinned"
  | "setPinned"
> & { onClearFilters(): void }) {
  return (
    <FilterBar
      label={t("controls.label")}
      search={
        <SearchField
          id="search"
          ref={searchElement}
          aria-label={t("search.label")}
          placeholder={t("search.placeholder")}
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          onClear={() => setSearch("")}
          clearLabel={t("search.clear")}
        />
      }
    >
      <Disclosure
        open={filtersOpen}
        onOpenChange={setFiltersOpen}
        title={
          activeFilters
            ? t("filters.active", { n: activeFilters })
            : t("filters.title")
        }
        actions={
          activeFilters > 0 && (
            <Button size="sm" variant="ghost" onClick={onClearFilters}>
              {t("filters.clear")}
            </Button>
          )
        }
      >
        <Toolbar label={t("filters.label")}>
          <Select
            label={t("status.label")}
            value={status}
            onValueChange={setStatus}
            options={[
              { value: "all", label: t("status.all") },
              { value: "pending", label: t("status.pending") },
              { value: "active", label: t("status.active") },
            ]}
          />
          <Select
            label={t("origin.label")}
            value={origin}
            onValueChange={setOrigin}
            options={[
              { value: "all", label: t("origin.all") },
              { value: "auto", label: t("origin.auto") },
              { value: "user", label: t("origin.user") },
            ]}
          />
          <Checkbox
            checked={pinned}
            onCheckedChange={setPinned}
            label={t("pinned.only")}
          />
        </Toolbar>
      </Disclosure>
    </FilterBar>
  );
}

function MemoryEditor({
  t,
  edit,
  feedback,
  recovering,
  aborted,
  scopeLabel,
  locked,
  pending,
  editorElement,
  setEdit,
  closeEditor,
  saveEdit,
  reconcile,
}: Pick<
  MemoryState,
  | "t"
  | "locked"
  | "pending"
  | "editorElement"
  | "setEdit"
  | "closeEditor"
  | "saveEdit"
  | "reconcile"
> & {
  edit: Exclude<Edit, { type: "delete" }>;
  feedback: ReactNode;
  recovering: boolean;
  aborted: boolean;
  scopeLabel: (value: Scope) => string;
}) {
  const title = edit.type === "add" ? t("add") : t("edit");
  const unchanged =
    edit.type === "update" && edit.content.trim() === edit.item.content;
  return (
    <RecordForm ref={editorElement} aria-label={title} data-focus-unit="">
      <RecordGroup>
        <RecordTitle>{title}</RecordTitle>
        {edit.type === "update" && (
          <RecordMeta>
            <span>{t(`scope.${scopeLabel(edit.item.scope)}`)}</span>
            <span>{edit.item.id}</span>
          </RecordMeta>
        )}
        <FormField label={t("content.label")}>
          <TextArea
            aria-label={t("content.label")}
            autoFocus
            rows={2}
            autosize
            style={EDITOR_STYLE}
            value={edit.content}
            pending={locked}
            onChange={(event) =>
              setEdit({ ...edit, content: event.target.value })
            }
          />
        </FormField>
      </RecordGroup>
      <FormActions label={t("save.actions.label")} feedback={feedback}>
        <Button
          disabled={locked}
          focusableWhenDisabled
          variant="ghost"
          onClick={() => closeEditor()}
        >
          {t("cancel")}
        </Button>
        <Button
          variant="primary"
          pending={pending}
          disabled={
            recovering ? aborted : locked || !edit.content.trim() || unchanged
          }
          focusableWhenDisabled={locked}
          onClick={recovering ? reconcile : saveEdit}
        >
          {recovering ? t("feedback.check") : t("save")}
        </Button>
      </FormActions>
    </RecordForm>
  );
}

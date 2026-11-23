import { type ComponentContext } from "@flame/component-sdk";
import { Refresh, Plus } from "@flame/icons/react";
import {
  Button,
  Panel,
  PanelHeader,
  IconButton,
  Icon,
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
import { type Scope } from "./types";
import { MemoryDeleteConfirmation } from "./MemoryDeleteConfirmation";
import { MemoryReport } from "./MemoryReport";
import { MemoryFeedback } from "./MemoryFeedback";
import { useMemory } from "./useMemory";
// Installed component bundles use host UI styles without StyleX compilation.
const EDITOR_STYLE = { maxHeight: "40dvh" };
export function Memory({ context }: { context: ComponentContext }) {
  const {
    t,
    edit,
    locked,
    setEdit,
    closeEditor,
    editorElement,
    scope,
    cwd,
    read,
    pending,
    observerSignal,
    load,
    beginEdit,
    setSearch,
    clearFilters,
    clearFeedback,
    setScope,
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
  } = useMemory(context);
  const recordCommand = unknown?.source === "record" ? unknown : undefined;
  const recovering = unknown?.source === "editor";
  const editFeedback = (editorFeedback || recovering) && (
    <MemoryFeedback
      canRestoreFocus={canRestoreFocus}
      feedback={editorFeedback}
      original={recovering ? unknown : undefined}
      pending={pending}
      reconcile={reconcile}
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
            <Metadata>
              {scope === "user"
                ? t("scope.user")
                : cwd
                  ? t("scope.project.path", { path: cwd })
                  : t("scope.none")}
            </Metadata>
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
                  disabled={cwd === undefined || pending}
                  onClick={() => {
                    const owner = observerSignal();
                    if (owner) void load(owner, current?.cursors);
                  }}
                />
                <Button
                  variant={edit ? "outline" : "primary"}
                  size="md"
                  disabled={locked || cwd === undefined || Boolean(edit)}
                  focusableWhenDisabled
                  onClick={(event) => {
                    beginEdit({
                      type: "add",
                      content: "",
                      opener: event.currentTarget,
                    });
                  }}
                >
                  <Icon glyph={Plus} size="sm" /> {t("add")}
                </Button>
              </>
            }
          >
            <Segmented
              ariaLabel={t("segment.label")}
              value={scope}
              options={[
                { value: "project", label: t("segment.project") },
                { value: "user", label: t("segment.user") },
              ]}
              onChange={(value) => {
                if (locked || edit) return;
                setSearch("");
                clearFilters();
                clearFeedback();
                setScope(value as Scope);
              }}
              disabled={locked || Boolean(edit)}
            />
          </Toolbar>
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
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={clearFiltersAndFocusSearch}
                  >
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
          confirm={
            recovering
              ? reconcile
              : () =>
                  change(
                    "flame.memory/delete.v3",
                    { id: edit.item.id, expectedRevision: edit.item.revision },
                    { action: "delete", id: edit.item.id },
                    "editor",
                  )
          }
          t={t}
        />
      ) : edit ? (
        <RecordForm
          ref={editorElement}
          aria-label={edit.type === "add" ? t("add") : t("edit")}
          data-focus-unit=""
        >
          <RecordGroup>
            <RecordTitle>
              {edit.type === "add" ? t("add") : t("edit")}
            </RecordTitle>
            {edit.type === "update" && (
              <RecordMeta>
                <span>{t(`scope.${edit.item.scope}`)}</span>
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
          <FormActions label={t("save.actions.label")} feedback={editFeedback}>
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
                recovering
                  ? context.signal.aborted
                  : locked ||
                    !edit.content.trim() ||
                    (edit.type === "update" &&
                      edit.content.trim() === edit.item.content)
              }
              focusableWhenDisabled={locked}
              onClick={
                recovering
                  ? reconcile
                  : () =>
                      edit.type === "add"
                        ? change(
                            "flame.memory/add.v3",
                            {
                              scope,
                              cwd: cwd!,
                              content: edit.content.trim(),
                            },
                            { action: "add", scope },
                            "editor",
                          )
                        : change(
                            "flame.memory/update.v3",
                            {
                              id: edit.item.id,
                              expectedRevision: edit.item.revision,
                              content: edit.content.trim(),
                            },
                            { action: "edit", id: edit.item.id },
                            "editor",
                          )
              }
            >
              {recovering ? t("feedback.check") : t("save")}
            </Button>
          </FormActions>
        </RecordForm>
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
                t={t}
              />
            }
            resultFocus={recordResult}
            authoring={Boolean(edit)}
            disabled={locked || read.loading || Boolean(read.error)}
            edit={(item, opener) => {
              beginEdit({
                type: "update",
                item: structuredClone(item),
                content: item.content,
                opener,
              });
            }}
            remove={(item, opener) => {
              beginEdit({
                type: "delete",
                item: structuredClone(item),
                opener,
              });
            }}
            command={(method, input, intent) =>
              change(method, input, intent, "record")
            }
            t={t}
          />
          {current &&
            (current.cursors.length > 1 ||
              Boolean(current.page.nextCursor)) && (
              <PanelFooter>
                <Toolbar label={t("pages.label")}>
                  <Button
                    pending={read.loading}
                    focusableWhenDisabled
                    disabled={pending || current.cursors.length === 1}
                    onClick={(event) => {
                      const owner = observerSignal();
                      if (owner)
                        void load(
                          owner,
                          current.cursors.slice(0, -1),
                          event.currentTarget,
                        );
                    }}
                  >
                    {t("pages.previous")}
                  </Button>
                  <Button
                    pending={read.loading}
                    focusableWhenDisabled
                    disabled={pending || !current.page.nextCursor}
                    onClick={(event) => {
                      const owner = observerSignal();
                      if (owner)
                        void load(
                          owner,
                          [...current.cursors, current.page.nextCursor],
                          event.currentTarget,
                        );
                    }}
                  >
                    {t("pages.next")}
                  </Button>
                </Toolbar>
              </PanelFooter>
            )}
        </>
      )}
    </Panel>
  );
}

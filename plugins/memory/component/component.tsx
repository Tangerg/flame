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
  RecordBody,
  RecordMeta,
  SearchField,
  FilterBar,
  Select,
  Toolbar,
  isImeKey,
} from "@flame/ui";
import { type Scope } from "./types";
import { MemoryReport } from "./MemoryReport";
import { MemoryFeedback } from "./MemoryFeedback";
import { useMemory } from "./useMemory";
export function Memory({ context }: { context: ComponentContext }) {
  const {
    t,
    edit,
    locked,
    setEdit,
    scope,
    cwd,
    read,
    pending,
    observerSignal,
    load,
    beginEdit,
    command,
    observing,
    setSearch,
    clearFilters,
    setEditorFeedback,
    setRecordFeedback,
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
    resolveOpener,
    editorFeedback,
    unknown,
    reconcile,
    change,
    current,
    recordFeedback,
  } = useMemory(context);
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
        if (!locked) setEdit(undefined);
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
                  disabled={read.loading || cwd === undefined || pending}
                  onClick={() => {
                    const owner = observerSignal();
                    if (owner) void load(owner);
                  }}
                />
                <Button
                  variant={edit ? "outline" : "primary"}
                  size="md"
                  disabled={locked || cwd === undefined || Boolean(edit)}
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
                if (edit || command.current || observing.current) return;
                setSearch("");
                clearFilters();
                setEditorFeedback(undefined);
                setRecordFeedback(undefined);
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
        <RecordForm
          aria-label={t("delete.confirm.label")}
          opener={resolveOpener}
        >
          <RecordTitle>{t("delete.title")}</RecordTitle>
          <RecordMeta>
            <span>
              {edit.item.Scope === "project"
                ? t("scope.project")
                : t("scope.user")}
            </span>
            <span>{edit.item.ID}</span>
          </RecordMeta>
          <RecordBody>{edit.item.Content}</RecordBody>
          <Metadata>{t("delete.notice")}</Metadata>
          <MemoryFeedback
            feedback={editorFeedback}
            original={unknown?.source === "editor" ? unknown : undefined}
            pending={pending}
            reconcile={reconcile}
            t={t}
          />
          <FormActions label={t("delete.actions.label")}>
            <Button
              variant="ghost"
              disabled={locked}
              // oxlint-disable-next-line jsx-a11y/no-autofocus
              autoFocus
              onClick={() => setEdit(undefined)}
            >
              {t("cancel")}
            </Button>
            <Button
              variant="tonal"
              tone="negative"
              disabled={locked}
              onClick={() =>
                change(
                  "flame.memory/delete.v2",
                  { ID: edit.item.ID, ExpectedRevision: edit.item.Revision },
                  t("command.delete", { id: edit.item.ID }),
                  "editor",
                )
              }
            >
              {t("delete.confirm")}
            </Button>
          </FormActions>
        </RecordForm>
      ) : edit ? (
        <RecordForm
          aria-label={edit.type === "add" ? t("add") : t("edit")}
          opener={resolveOpener}
        >
          <RecordTitle>
            {edit.type === "add" ? t("add") : t("edit")}
          </RecordTitle>
          {edit.type === "update" && (
            <RecordMeta>
              <span>{edit.item.ID}</span>
            </RecordMeta>
          )}
          <FormField label={t("content.label")}>
            <TextArea
              aria-label={t("content.label")}
              // oxlint-disable-next-line jsx-a11y/no-autofocus
              autoFocus
              rows={6}
              value={edit.content}
              pending={locked}
              onChange={(event) =>
                setEdit({ ...edit, content: event.target.value })
              }
            />
          </FormField>
          <MemoryFeedback
            feedback={editorFeedback}
            original={unknown?.source === "editor" ? unknown : undefined}
            pending={pending}
            reconcile={reconcile}
            t={t}
          />
          <FormActions label={t("save.actions.label")}>
            <Button
              disabled={locked}
              variant="ghost"
              onClick={() => setEdit(undefined)}
            >
              {t("cancel")}
            </Button>
            <Button
              variant="primary"
              disabled={
                locked ||
                !edit.content.trim() ||
                (edit.type === "update" &&
                  edit.content.trim() === edit.item.Content)
              }
              onClick={() =>
                edit.type === "add"
                  ? change(
                      "flame.memory/add.v2",
                      {
                        Scope: scope,
                        CWD: cwd!,
                        Content: edit.content.trim(),
                      },
                      t(`command.add.${scope}`),
                      "editor",
                    )
                  : change(
                      "flame.memory/update.v2",
                      {
                        ID: edit.item.ID,
                        ExpectedRevision: edit.item.Revision,
                        Content: edit.content.trim(),
                      },
                      t("command.edit", { id: edit.item.ID }),
                      "editor",
                    )
              }
            >
              {t("save")}
            </Button>
          </FormActions>
        </RecordForm>
      ) : null}
      {(current || recordFeedback || unknown?.source === "record") && (
        <>
          <MemoryReport
            page={current?.page}
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
                feedback={recordFeedback}
                original={unknown?.source === "record" ? unknown : undefined}
                pending={pending}
                reconcile={reconcile}
                t={t}
              />
            }
            authoring={Boolean(edit)}
            disabled={locked || read.loading || Boolean(read.error)}
            edit={(item, opener) => {
              beginEdit({
                type: "update",
                item: structuredClone(item),
                content: item.Content,
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
            command={change}
            t={t}
          />
          {current &&
            (current.cursors.length > 1 ||
              Boolean(current.page.NextCursor)) && (
              <PanelFooter>
                <Metadata>
                  {t("footer.page", {
                    page: current.cursors.length,
                    count: current.page.Rows.length,
                  })}
                </Metadata>{" "}
                <Toolbar label={t("pages.label")}>
                  <Button
                    disabled={
                      read.loading || pending || current.cursors.length === 1
                    }
                    onClick={() => {
                      const owner = observerSignal();
                      if (owner) void load(owner, current.cursors.slice(0, -1));
                    }}
                  >
                    {t("pages.previous")}
                  </Button>
                  <Button
                    disabled={
                      read.loading || pending || !current.page.NextCursor
                    }
                    onClick={() => {
                      const owner = observerSignal();
                      if (owner)
                        void load(owner, [
                          ...current.cursors,
                          current.page.NextCursor,
                        ]);
                    }}
                  >
                    {t("pages.next")}
                  </Button>
                </Toolbar>{" "}
              </PanelFooter>
            )}
        </>
      )}
    </Panel>
  );
}

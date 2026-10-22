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
  return (
    <Panel
      label="Agent memory"
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
          title="Agent memory"
          context={
            <Metadata>
              {scope === "user"
                ? "User memory"
                : cwd
                  ? `Project memory · ${cwd}`
                  : "Choose a Session with a workspace, or select User memory."}
            </Metadata>
          }
        />
      }
      controls={
        <>
          <Toolbar
            label="Memory navigation"
            actions={
              <>
                <IconButton
                  icon={Refresh}
                  aria-label="Refresh"
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
                  <Icon glyph={Plus} size="sm" /> Add memory
                </Button>
              </>
            }
          >
            <Segmented
              ariaLabel="Memory scope"
              value={scope}
              options={[
                { value: "project", label: "Current project" },
                { value: "user", label: "User" },
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
            label="Memory controls"
            search={
              <SearchField
                id="search"
                ref={searchElement}
                aria-label="Search this page"
                placeholder="Text or ID on this page"
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                onClear={() => setSearch("")}
                clearLabel="Clear search"
              />
            }
          >
            <Disclosure
              open={filtersOpen}
              onOpenChange={setFiltersOpen}
              title={
                activeFilters ? `Filters · ${activeFilters} active` : "Filters"
              }
              actions={
                activeFilters > 0 && (
                  <Button size="sm" variant="ghost" onClick={clearFilters}>
                    Clear filters
                  </Button>
                )
              }
            >
              <Toolbar label="Memory filters">
                <Select
                  label="Memory status"
                  value={status}
                  onValueChange={setStatus}
                  options={[
                    { value: "all", label: "All statuses" },
                    { value: "pending", label: "Pending review" },
                    { value: "active", label: "Active" },
                  ]}
                />
                <Select
                  label="Memory origin"
                  value={origin}
                  onValueChange={setOrigin}
                  options={[
                    { value: "all", label: "All origins" },
                    { value: "auto", label: "Mined" },
                    { value: "user", label: "User authored" },
                  ]}
                />
                <Checkbox
                  checked={pinned}
                  onCheckedChange={setPinned}
                  label="Pinned only"
                />
              </Toolbar>
            </Disclosure>
          </FilterBar>
        </>
      }
    >
      {read.loading && <p role="status">Reading recorded memory…</p>}
      {read.error && <p role="alert">{read.error}</p>}
      {edit?.type === "delete" ? (
        <RecordForm aria-label="Confirm memory deletion" opener={resolveOpener}>
          <RecordTitle>Delete this memory?</RecordTitle>
          <RecordMeta>
            <span>
              {edit.item.Scope === "project" ? "Project memory" : "User memory"}
            </span>
            <span>{edit.item.ID}</span>
          </RecordMeta>
          <RecordBody>{edit.item.Content}</RecordBody>
          <Metadata>
            This memory will no longer be available to the agent.
          </Metadata>
          <MemoryFeedback
            feedback={editorFeedback}
            original={unknown?.source === "editor" ? unknown : undefined}
            pending={pending}
            reconcile={reconcile}
          />
          <FormActions label="Memory deletion confirmation">
            <Button
              variant="ghost"
              disabled={locked}
              // oxlint-disable-next-line jsx-a11y/no-autofocus
              autoFocus
              onClick={() => setEdit(undefined)}
            >
              Cancel
            </Button>
            <Button
              variant="tonal"
              tone="negative"
              disabled={locked}
              onClick={() =>
                change(
                  "flame.memory/delete.v1",
                  { ID: edit.item.ID },
                  `Delete ${edit.item.ID}`,
                  "editor",
                )
              }
            >
              Delete memory
            </Button>
          </FormActions>
        </RecordForm>
      ) : edit ? (
        <RecordForm
          aria-label={edit.type === "add" ? "Add memory" : "Edit memory"}
          opener={resolveOpener}
        >
          <RecordTitle>
            {edit.type === "add" ? "Add memory" : "Edit memory"}
          </RecordTitle>
          {edit.type === "update" && (
            <RecordMeta>
              <span>{edit.item.ID}</span>
            </RecordMeta>
          )}
          <FormField label="Memory content">
            <TextArea
              aria-label="Memory content"
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
          />
          <FormActions label="Save memory content">
            <Button
              disabled={locked}
              variant="ghost"
              onClick={() => setEdit(undefined)}
            >
              Cancel
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
                      "flame.memory/add.v1",
                      {
                        Scope: scope,
                        CWD: cwd!,
                        Content: edit.content.trim(),
                      },
                      `Add ${scope} memory`,
                      "editor",
                    )
                  : change(
                      "flame.memory/update.v1",
                      {
                        ID: edit.item.ID,
                        Content: edit.content.trim(),
                      },
                      `Edit ${edit.item.ID}`,
                      "editor",
                    )
              }
            >
              Save
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
            feedback={
              <MemoryFeedback
                feedback={recordFeedback}
                original={unknown?.source === "record" ? unknown : undefined}
                pending={pending}
                reconcile={reconcile}
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
          />
          {current &&
            (current.cursors.length > 1 ||
              Boolean(current.page.NextCursor)) && (
              <PanelFooter>
                <Metadata>
                  Page {current.cursors.length} · {current.page.Rows.length}{" "}
                  recorded items
                </Metadata>{" "}
                <Toolbar label="Memory pages">
                  <Button
                    disabled={
                      read.loading || pending || current.cursors.length === 1
                    }
                    onClick={() => {
                      const owner = observerSignal();
                      if (owner) void load(owner, current.cursors.slice(0, -1));
                    }}
                  >
                    Previous page
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
                    Next page
                  </Button>
                </Toolbar>{" "}
              </PanelFooter>
            )}
        </>
      )}
    </Panel>
  );
}

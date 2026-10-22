import { CalendarClock, Refresh, Plus, ChevronDown } from "@flame/icons/react";
import { type ComponentContext } from "@flame/component-sdk";

import {
  Badge,
  Button,
  Checkbox,
  EmptyState,
  Facts,
  ListRow,
  Metadata,
  ModalDialog,
  Panel,
  PanelHeader,
  PanelSection,
  PanelFooter,
  FormRow,
  FormSection,
  ActionMenu,
  IconButton,
  Icon,
  Disclosure,
  FormField,
  RecordBody,
  RecordGroup,
  RecordHeading,
  SearchField,
  FilterBar,
  Select,
  TextArea,
  TextField,
  Toolbar,
} from "@flame/ui";
import { templates } from "./templates";

import { useSchedules, selection, enabled, hasTime } from "./useSchedules";
export function Schedules({ context }: { context: ComponentContext }) {
  const {
    notice,
    command,
    reconcile,
    searchElement,
    search,
    setSearch,
    editorOpen,
    actionsDisabled,
    add,
    status,
    setStatus,
    read,
    busy,
    refresh,
    liveError,
    edit,
    titleElement,
    resolveOpener,
    cancelTask,
    save,
    canSave,
    patch,
    selectedKey,
    models,
    selectedModel,
    currentModels,
    setModelRefresh,
    deleting,
    deleteCancel,
    submit,
    page,
    rows,
    matching,
    disabled,
    taskOpener,
    setNotice,
    setEdit,
    setDeleting,
  } = useSchedules(context);
  const feedback = (
    <>
      {notice && <p role="status">{notice}</p>}
      {command && (
        <section aria-label="Schedule command status">
          <p role={command.uncertain ? "alert" : "status"}>
            {command.uncertain
              ? "The original outcome is not confirmed. Reconcile it before issuing another command."
              : `${command.label}…`}
          </p>
          {command.error && <RecordBody>{command.error}</RecordBody>}
          {command.uncertain && (
            <>
              <Metadata>
                Original command: {command.promise.idempotencyKey}
              </Metadata>
              <Button onClick={reconcile}>Reconcile original command</Button>
            </>
          )}
        </section>
      )}
    </>
  );
  return (
    <Panel
      label="Schedules"
      header={<PanelHeader title="Schedules" />}
      controls={
        <FilterBar
          label="Schedule controls"
          search={
            <SearchField
              ref={searchElement}
              size="lg"
              aria-label="Search this page"
              placeholder="Title or instructions"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              onClear={() => setSearch("")}
              clearLabel="Clear search"
            />
          }
          actions={
            <Toolbar label="Schedule actions">
              <Button
                variant={editorOpen ? "outline" : "primary"}
                size="lg"
                disabled={actionsDisabled}
                onClick={(event) => add(event.currentTarget)}
              >
                <Icon glyph={Plus} size="sm" /> Add schedule
              </Button>
              <ActionMenu
                label="Schedule templates"
                disabled={actionsDisabled}
                triggerContent={
                  <>
                    Templates <Icon glyph={ChevronDown} size="sm" />
                  </>
                }
                items={templates.map((template) => ({
                  label: template.title,
                  onSelect: (opener) => add(opener, template),
                }))}
              />
            </Toolbar>
          }
        >
          <Select
            size="lg"
            label="Schedule status"
            value={status}
            onValueChange={setStatus}
            options={[
              { value: "all", label: "All statuses" },
              { value: "enabled", label: "Enabled" },
              { value: "disabled", label: "Disabled" },
            ]}
          />
          <IconButton
            icon={Refresh}
            aria-label="Refresh"
            size="md"
            pending={read.loading}
            disabled={busy}
            onClick={() => void refresh()}
          />
        </FilterBar>
      }
    >
      {read.loading && <p role="status">Reading schedules…</p>}
      {read.error && (
        <p role="alert">
          Schedules could not be read: {read.error}.{" "}
          {read.accepted && "The last accepted page is retained. "}
          Use Refresh to try again.
        </p>
      )}
      {liveError && (
        <p role="alert">
          Live refresh is unavailable: {liveError}. Use Refresh to read the
          current schedules.
        </p>
      )}
      {!editorOpen && feedback}
      {edit && (
        <ModalDialog
          open
          title={
            edit.basis
              ? `Edit ${edit.basis.Title || "schedule"}`
              : "New schedule"
          }
          description={
            edit.basis
              ? `Captured revision: ${String(edit.basis.Revision)}`
              : "Configure the task and when it should run."
          }
          closeLabel="Close schedule editor"
          size="lg"
          initialFocus={titleElement}
          finalFocus={resolveOpener}
          dismissible={!busy}
          onOpenChange={(open) => {
            if (!open) cancelTask();
          }}
          onSubmit={(event) => {
            event.preventDefault();
            save();
          }}
          auxiliaryAction={feedback}
          footer={
            <>
              <Button variant="ghost" disabled={busy} onClick={cancelTask}>
                Cancel
              </Button>
              <Button variant="primary" type="submit" disabled={!canSave}>
                Save schedule
              </Button>
            </>
          }
        >
          <FormSection
            title="Task"
            description="What the agent should do when this schedule runs."
          >
            <FormField label="Title">
              <TextField
                ref={titleElement}
                size="lg"
                aria-label="Title"
                placeholder="Title"
                value={edit.draft.Title}
                disabled={busy}
                onChange={(event) => patch("Title", event.target.value)}
              />
            </FormField>
            <FormField label="Instructions">
              <TextArea
                aria-label="Instructions"
                placeholder="Instructions"
                value={edit.draft.Instructions}
                disabled={busy}
                rows={4}
                onChange={(event) => patch("Instructions", event.target.value)}
              />
            </FormField>
          </FormSection>
          <FormSection title="Timing and workspace">
            <FormRow>
              <FormField label="Cron expression">
                <TextField
                  size="lg"
                  aria-label="Cron expression"
                  placeholder="0 9 * * 1"
                  value={edit.draft.Cron}
                  disabled={busy}
                  onChange={(event) => patch("Cron", event.target.value)}
                />
              </FormField>
              <FormField label="Workspace path">
                <TextField
                  size="lg"
                  aria-label="Workspace path"
                  placeholder="Empty uses the Runtime default"
                  value={edit.draft.CWD}
                  disabled={busy}
                  onChange={(event) => patch("CWD", event.target.value)}
                />
              </FormField>
            </FormRow>
            <Checkbox
              label="Enabled"
              checked={edit.draft.Enabled}
              disabled={busy}
              onCheckedChange={(value) => patch("Enabled", value)}
            />
          </FormSection>
          <FormSection
            title="Execution"
            description="Choose the model and reasoning used for future runs."
          >
            <FormRow>
              <FormField label="Model">
                <Select
                  size="lg"
                  label="Model"
                  value={selectedKey}
                  disabled={busy}
                  options={[
                    { value: "default", label: "Runtime default model" },
                    ...models
                      .filter((model) => !model.deprecated)
                      .map((model) => ({
                        value: JSON.stringify([model.provider, model.id]),
                        label: `${model.provider} / ${model.displayName ?? model.id}`,
                      })),
                    ...(selectedKey !== "default" &&
                    (!selectedModel || selectedModel.deprecated)
                      ? [
                          {
                            value: selectedKey,
                            label: `${edit.draft.ModelSelection.provider} / ${edit.draft.ModelSelection.model} (${selectedModel?.deprecated ? "deprecated" : "unavailable"})`,
                          },
                        ]
                      : []),
                  ]}
                  onValueChange={(key) =>
                    patch(
                      "ModelSelection",
                      selection(
                        models.find(
                          (model) =>
                            JSON.stringify([model.provider, model.id]) === key,
                        ),
                      ),
                    )
                  }
                />
              </FormField>
              <FormField label="Reasoning effort">
                <Select
                  size="lg"
                  label="Reasoning effort"
                  value={edit.draft.ModelSelection.reasoningEffort || "default"}
                  disabled={busy || !edit.draft.ModelSelection.model}
                  options={[
                    { value: "default", label: "Provider default" },
                    ...(selectedModel?.capabilities?.reasoningLevels ?? []).map(
                      (value) => ({
                        value,
                        label: value,
                      }),
                    ),
                    ...(edit.draft.ModelSelection.reasoningEffort &&
                    !selectedModel?.capabilities?.reasoningLevels?.includes(
                      edit.draft.ModelSelection.reasoningEffort,
                    )
                      ? [
                          {
                            value: edit.draft.ModelSelection.reasoningEffort,
                            label: `${edit.draft.ModelSelection.reasoningEffort} (unavailable)`,
                          },
                        ]
                      : []),
                  ]}
                  onValueChange={(value) =>
                    patch("ModelSelection", {
                      ...edit.draft.ModelSelection,
                      reasoningEffort: value === "default" ? "" : value,
                    })
                  }
                />
              </FormField>
            </FormRow>
            {currentModels?.error && (
              <p role="alert">
                Model choices could not be read: {currentModels.error}
              </p>
            )}
            <Button
              disabled={busy || currentModels?.loading}
              onClick={() => setModelRefresh((value) => value + 1)}
            >
              Refresh model choices
            </Button>
          </FormSection>
        </ModalDialog>
      )}
      {deleting && (
        <ModalDialog
          open
          role="alertdialog"
          title={`Delete ${deleting.item.Title || deleting.item.ID}?`}
          description="Future firings will stop. Captured work and accepted Runs are retained."
          closeLabel="Close deletion confirmation"
          size="sm"
          initialFocus={deleteCancel}
          finalFocus={resolveOpener}
          dismissible={!busy}
          onOpenChange={(open) => {
            if (!open) cancelTask();
          }}
          auxiliaryAction={feedback}
          footer={
            <>
              <Button
                ref={deleteCancel}
                variant="ghost"
                disabled={busy}
                onClick={cancelTask}
              >
                Cancel
              </Button>
              <Button
                variant="tonal"
                tone="negative"
                disabled={busy}
                onClick={() =>
                  submit(
                    "flame.schedules/delete.v1",
                    { ID: deleting.item.ID },
                    "Delete schedule",
                  )
                }
              >
                Confirm deletion
              </Button>
            </>
          }
        />
      )}
      <PanelSection
        title="Saved schedules"
        description={
          page ? (
            <>
              {rows.filter(enabled).length} enabled ·{" "}
              {rows.filter((item) => !enabled(item)).length} disabled on this
              page
            </>
          ) : undefined
        }
      >
        <RecordGroup variant="list" id="items" aria-label="Saved schedules">
          {page && !matching.length && (
            <EmptyState
              icon={CalendarClock}
              size="compact"
              title={
                rows.length
                  ? "No matching schedules on this page."
                  : "No schedules saved."
              }
              sub={
                rows.length
                  ? "Clear the search and filters."
                  : "Use Add schedule to save recurring work."
              }
            />
          )}
          {matching.map((item) => (
            <ListRow key={item.ID}>
              <RecordHeading
                title={item.Title || "Untitled schedule"}
                actions={
                  <ActionMenu
                    label={`More actions for ${item.Title || "Untitled schedule"}`}
                    disabled={disabled}
                    items={[
                      {
                        label: "Run now",
                        onSelect: () =>
                          submit(
                            "flame.schedules/runNow.v1",
                            { ID: item.ID },
                            "Run schedule now",
                          ),
                      },
                      {
                        label: "Edit",
                        onSelect: (opener) => {
                          taskOpener.current = opener;
                          setNotice(undefined);
                          setEdit({
                            basis: structuredClone(item),
                            draft: {
                              Title: item.Title,
                              Instructions: item.Instructions,
                              Cron: item.Cron,
                              CWD: item.CWD,
                              ModelSelection: structuredClone(
                                item.ModelSelection,
                              ),
                              Enabled: enabled(item),
                            },
                          });
                          setDeleting(undefined);
                        },
                      },
                      {
                        label: enabled(item) ? "Disable" : "Enable",
                        onSelect: () =>
                          submit(
                            "flame.schedules/update.v1",
                            {
                              ID: item.ID,
                              ExpectedRevision: item.Revision,
                              Enabled: !enabled(item),
                              ModelSelection: {},
                            },
                            enabled(item)
                              ? "Disable schedule"
                              : "Enable schedule",
                          ),
                      },
                      {
                        label: "Delete",
                        tone: "negative",
                        onSelect: (opener) => {
                          taskOpener.current = opener;
                          setNotice(undefined);
                          setDeleting({ item: structuredClone(item) });
                          setEdit(undefined);
                        },
                      },
                    ]}
                  />
                }
                status={
                  <Badge tone={enabled(item) ? "success" : "neutral"}>
                    {enabled(item) ? "Enabled" : "Disabled"}
                  </Badge>
                }
              />
              <RecordBody>{item.Instructions}</RecordBody>
              <Metadata>
                Next firing ·{" "}
                {hasTime(item.NextRunAt) ? (
                  <time dateTime={item.NextRunAt}>
                    {new Intl.DateTimeFormat(navigator.language, {
                      dateStyle: "medium",
                      timeStyle: "short",
                    }).format(new Date(item.NextRunAt))}
                  </time>
                ) : (
                  "Not scheduled"
                )}
              </Metadata>
              <Disclosure title="Schedule details">
                <Facts
                  values={[
                    ["Cron", item.Cron],
                    ["Schedule", item.ID],
                    ["Revision", String(item.Revision)],
                    ["Workspace", item.CWD || "Runtime default"],
                    [
                      "Model",
                      item.ModelSelection.provider && item.ModelSelection.model
                        ? `${item.ModelSelection.provider}/${item.ModelSelection.model}`
                        : "Runtime default",
                    ],
                    [
                      "Reasoning effort",
                      item.ModelSelection.reasoningEffort || undefined,
                    ],
                    [
                      "Last firing",
                      hasTime(item.LastRunAt) ? item.LastRunAt : "Never fired",
                    ],
                    ["Created", item.CreatedAt],
                  ]}
                />
              </Disclosure>
            </ListRow>
          ))}
        </RecordGroup>
        {(Boolean(read.accepted?.history.length) ||
          Boolean(page?.NextCursor)) && (
          <PanelFooter>
            <Toolbar label="Schedule pages">
              <Button
                disabled={
                  read.loading || busy || !read.accepted?.history.length
                }
                onClick={() =>
                  void refresh(read.accepted!.history.slice(0, -1))
                }
              >
                Previous
              </Button>
              <Button
                disabled={read.loading || busy || !page?.NextCursor}
                onClick={() =>
                  void refresh([
                    ...(read.accepted?.history ?? []),
                    page!.NextCursor,
                  ])
                }
              >
                Next
              </Button>
            </Toolbar>
          </PanelFooter>
        )}
      </PanelSection>
    </Panel>
  );
}

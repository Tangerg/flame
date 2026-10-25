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
    t,
    dateTime,
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
        <section aria-label={t("command.status.label")}>
          <p role={command.uncertain ? "alert" : "status"}>
            {command.uncertain
              ? t("command.uncertain")
              : t("command.progress", { label: command.label })}
          </p>
          {command.error && <RecordBody>{command.error}</RecordBody>}
          {command.uncertain && (
            <>
              <Metadata>
                {t("command.original", {
                  key: command.promise.idempotencyKey,
                })}
              </Metadata>
              <Button onClick={reconcile}>{t("command.reconcile")}</Button>
            </>
          )}
        </section>
      )}
    </>
  );
  return (
    <Panel
      label={t("panel.title")}
      header={<PanelHeader title={t("panel.title")} />}
      controls={
        <FilterBar
          label={t("controls.label")}
          search={
            <SearchField
              ref={searchElement}
              size="lg"
              aria-label={t("search.label")}
              placeholder={t("search.placeholder")}
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              onClear={() => setSearch("")}
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
                    {t("templates.trigger")}{" "}
                    <Icon glyph={ChevronDown} size="sm" />
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
            onValueChange={setStatus}
            options={[
              { value: "all", label: t("status.all") },
              { value: "enabled", label: t("status.enabled") },
              { value: "disabled", label: t("status.disabled") },
            ]}
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
      }
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
        <ModalDialog
          open
          title={
            edit.basis
              ? t("editor.edit.title", {
                  title: edit.basis.Title || t("editor.edit.untitled"),
                })
              : t("editor.new.title")
          }
          description={
            edit.basis
              ? t("editor.edit.description", {
                  revision: String(edit.basis.Revision),
                })
              : t("editor.new.description")
          }
          closeLabel={t("editor.close")}
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
                {t("cancel")}
              </Button>
              <Button variant="primary" type="submit" disabled={!canSave}>
                {t("command.save")}
              </Button>
            </>
          }
        >
          <FormSection
            title={t("editor.section.task.title")}
            description={t("editor.section.task.description")}
          >
            <FormField label={t("field.title")}>
              <TextField
                ref={titleElement}
                size="lg"
                aria-label={t("field.title")}
                value={edit.draft.Title}
                disabled={busy}
                onChange={(event) => patch("Title", event.target.value)}
              />
            </FormField>
            <FormField label={t("field.instructions")}>
              <TextArea
                aria-label={t("field.instructions")}
                value={edit.draft.Instructions}
                disabled={busy}
                rows={4}
                onChange={(event) => patch("Instructions", event.target.value)}
              />
            </FormField>
          </FormSection>
          <FormSection title={t("editor.section.timing.title")}>
            <FormRow>
              <FormField label={t("field.cron")}>
                <TextField
                  size="lg"
                  aria-label={t("field.cron")}
                  placeholder="0 9 * * 1"
                  value={edit.draft.Cron}
                  disabled={busy}
                  onChange={(event) => patch("Cron", event.target.value)}
                />
              </FormField>
              <FormField label={t("field.workspace")}>
                <TextField
                  size="lg"
                  aria-label={t("field.workspace")}
                  placeholder={t("field.workspace.placeholder")}
                  value={edit.draft.CWD}
                  disabled={busy}
                  onChange={(event) => patch("CWD", event.target.value)}
                />
              </FormField>
            </FormRow>
            <Checkbox
              label={t("status.enabled")}
              checked={edit.draft.Enabled}
              disabled={busy}
              onCheckedChange={(value) => patch("Enabled", value)}
            />
          </FormSection>
          <FormSection
            title={t("editor.section.execution.title")}
            description={t("editor.section.execution.description")}
          >
            <FormRow>
              <FormField label={t("field.model")}>
                <Select
                  size="lg"
                  label={t("field.model")}
                  value={selectedKey}
                  disabled={busy}
                  options={[
                    { value: "default", label: t("model.default") },
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
                            label: t("model.existing", {
                              provider: edit.draft.ModelSelection.provider,
                              model: edit.draft.ModelSelection.model,
                              state: selectedModel?.deprecated
                                ? t("model.deprecated")
                                : t("model.unavailable"),
                            }),
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
              <FormField label={t("field.reasoning")}>
                <Select
                  size="lg"
                  label={t("field.reasoning")}
                  value={edit.draft.ModelSelection.reasoningEffort || "default"}
                  disabled={busy || !edit.draft.ModelSelection.model}
                  options={[
                    { value: "default", label: t("reasoning.default") },
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
                            label: t("reasoning.unavailable", {
                              effort: edit.draft.ModelSelection.reasoningEffort,
                            }),
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
                {t("models.error", { error: currentModels.error })}
              </p>
            )}
            <Button
              disabled={busy || currentModels?.loading}
              onClick={() => setModelRefresh((value) => value + 1)}
            >
              {t("models.refresh")}
            </Button>
          </FormSection>
        </ModalDialog>
      )}
      {deleting && (
        <ModalDialog
          open
          role="alertdialog"
          title={t("delete.title", {
            name: deleting.item.Title || deleting.item.ID,
          })}
          description={t("delete.description")}
          closeLabel={t("delete.close")}
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
                {t("cancel")}
              </Button>
              <Button
                variant="tonal"
                tone="negative"
                disabled={busy}
                onClick={() =>
                  submit(
                    "flame.schedules/delete.v1",
                    { ID: deleting.item.ID },
                    t("command.delete"),
                  )
                }
              >
                {t("delete.confirm")}
              </Button>
            </>
          }
        />
      )}
      <PanelSection
        title={t("catalog.title")}
        description={
          page
            ? t("catalog.summary", {
                enabled: rows.filter(enabled).length,
                disabled: rows.filter((item) => !enabled(item)).length,
              })
            : undefined
        }
      >
        <RecordGroup
          variant="list"
          id="items"
          aria-label={t("catalog.title")}
          hidden={!page}
        >
          {page && !matching.length && (
            <EmptyState
              icon={CalendarClock}
              size="compact"
              title={
                rows.length ? t("empty.filtered.title") : t("empty.none.title")
              }
              sub={rows.length ? undefined : t("empty.none.sub")}
              action={
                rows.length ? (
                  <Button
                    variant="outline"
                    onClick={() => {
                      setSearch("");
                      setStatus("all");
                      searchElement.current?.focus();
                    }}
                  >
                    {t("empty.filtered.action")}
                  </Button>
                ) : undefined
              }
            />
          )}
          {matching.map((item) => (
            <ListRow key={item.ID}>
              <RecordHeading
                title={item.Title || t("untitled")}
                actions={
                  <ActionMenu
                    label={t("actions.more", {
                      name: item.Title || t("untitled"),
                    })}
                    disabled={disabled}
                    items={[
                      {
                        label: t("action.runNow"),
                        onSelect: () =>
                          submit(
                            "flame.schedules/runNow.v1",
                            { ID: item.ID },
                            t("command.runNow"),
                          ),
                      },
                      {
                        label: t("action.edit"),
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
                        label: enabled(item)
                          ? t("action.disable")
                          : t("action.enable"),
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
                              ? t("command.disable")
                              : t("command.enable"),
                          ),
                      },
                      {
                        label: t("action.delete"),
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
                    {enabled(item) ? t("status.enabled") : t("status.disabled")}
                  </Badge>
                }
              />
              <RecordBody>{item.Instructions}</RecordBody>
              <Metadata>
                {t("nextFiring.label")} ·{" "}
                {hasTime(item.NextRunAt) ? (
                  <time dateTime={item.NextRunAt}>
                    {dateTime.format(new Date(item.NextRunAt))}
                  </time>
                ) : (
                  t("nextFiring.none")
                )}
              </Metadata>
              <Disclosure title={t("details.title")}>
                <Facts
                  values={[
                    [t("fact.cron"), item.Cron],
                    [t("fact.schedule"), item.ID],
                    [t("fact.revision"), String(item.Revision)],
                    [t("fact.workspace"), item.CWD || t("runtimeDefault")],
                    [
                      t("field.model"),
                      item.ModelSelection.provider && item.ModelSelection.model
                        ? `${item.ModelSelection.provider}/${item.ModelSelection.model}`
                        : t("runtimeDefault"),
                    ],
                    [
                      t("field.reasoning"),
                      item.ModelSelection.reasoningEffort || undefined,
                    ],
                    [
                      t("fact.lastFiring"),
                      hasTime(item.LastRunAt)
                        ? dateTime.format(new Date(item.LastRunAt))
                        : t("fact.neverFired"),
                    ],
                    [
                      t("fact.created"),
                      dateTime.format(new Date(item.CreatedAt)),
                    ],
                  ]}
                />
              </Disclosure>
            </ListRow>
          ))}
        </RecordGroup>
        {(Boolean(read.accepted?.history.length) ||
          Boolean(page?.NextCursor)) && (
          <PanelFooter>
            <Toolbar label={t("pages.label")}>
              <Button
                disabled={
                  read.loading || busy || !read.accepted?.history.length
                }
                onClick={() =>
                  void refresh(read.accepted!.history.slice(0, -1))
                }
              >
                {t("pages.previous")}
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
                {t("pages.next")}
              </Button>
            </Toolbar>
          </PanelFooter>
        )}
      </PanelSection>
    </Panel>
  );
}

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
  FormActions,
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

import { useSchedules, selection, enabled } from "./useSchedules";
export function Schedules({ context }: { context: ComponentContext }) {
  const openSession = context.openSession;
  const {
    t,
    dateTime,
    notice,
    feedbackElement,
    command,
    reconcile,
    searchElement,
    search,
    filterPage,
    editorOpen,
    actionsDisabled,
    add,
    status,
    read,
    busy,
    refresh,
    summary,
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
            onValueChange={(value) => filterPage(search, value)}
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
                  title: edit.basis.title || t("editor.edit.untitled"),
                })
              : t("editor.new.title")
          }
          description={
            edit.basis
              ? t("editor.edit.description", {
                  revision: String(edit.basis.revision),
                })
              : undefined
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
              <Button
                variant="primary"
                type="submit"
                focusableWhenDisabled={busy}
                disabled={!canSave}
              >
                {t("command.save")}
              </Button>
            </>
          }
        >
          <FormField label={t("field.title")}>
            <TextField
              ref={titleElement}
              size="lg"
              aria-label={t("field.title")}
              value={edit.draft.title}
              disabled={busy}
              onChange={(event) => patch("title", event.target.value)}
            />
          </FormField>
          <FormField label={t("field.instructions")}>
            <TextArea
              aria-label={t("field.instructions")}
              value={edit.draft.instructions}
              disabled={busy}
              rows={4}
              onChange={(event) => patch("instructions", event.target.value)}
            />
          </FormField>
          <FormSection title={t("editor.section.timing.title")}>
            <FormRow>
              <FormField label={t("field.cron")}>
                <TextField
                  size="lg"
                  aria-label={t("field.cron")}
                  placeholder="0 9 * * 1"
                  value={edit.draft.cron}
                  disabled={busy}
                  onChange={(event) => patch("cron", event.target.value)}
                />
              </FormField>
              <FormField label={t("field.timeZone")}>
                <TextField
                  size="lg"
                  aria-label={t("field.timeZone")}
                  value={edit.draft.timeZone}
                  disabled={busy}
                  onChange={(event) => patch("timeZone", event.target.value)}
                />
              </FormField>
              <FormField label={t("field.workspace")}>
                <TextField
                  size="lg"
                  aria-label={t("field.workspace")}
                  placeholder={t("field.workspace.placeholder")}
                  value={edit.draft.cwd}
                  disabled={busy}
                  onChange={(event) => patch("cwd", event.target.value)}
                />
              </FormField>
            </FormRow>
            <Checkbox
              label={t("status.enabled")}
              checked={edit.draft.enabled}
              disabled={busy}
              onCheckedChange={(value) => patch("enabled", value)}
            />
          </FormSection>
          <FormSection title={t("editor.section.execution.title")}>
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
                              provider: edit.draft.modelSelection.provider,
                              model: edit.draft.modelSelection.model,
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
                      "modelSelection",
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
                  value={edit.draft.modelSelection.reasoningEffort || "default"}
                  disabled={busy || !edit.draft.modelSelection.model}
                  options={[
                    { value: "default", label: t("reasoning.default") },
                    ...(selectedModel?.capabilities?.reasoningLevels ?? []).map(
                      (value) => ({
                        value,
                        label: value,
                      }),
                    ),
                    ...(edit.draft.modelSelection.reasoningEffort &&
                    !selectedModel?.capabilities?.reasoningLevels?.includes(
                      edit.draft.modelSelection.reasoningEffort,
                    )
                      ? [
                          {
                            value: edit.draft.modelSelection.reasoningEffort,
                            label: t("reasoning.unavailable", {
                              effort: edit.draft.modelSelection.reasoningEffort,
                            }),
                          },
                        ]
                      : []),
                  ]}
                  onValueChange={(value) =>
                    patch("modelSelection", {
                      ...edit.draft.modelSelection,
                      reasoningEffort: value === "default" ? "" : value,
                    })
                  }
                />
              </FormField>
            </FormRow>
            <FormActions
              label={t("models.refresh")}
              feedback={
                currentModels?.error && (
                  <p role="alert">
                    {t("models.error", { error: currentModels.error })}
                  </p>
                )
              }
            >
              <Button
                pending={currentModels?.loading && !busy}
                aria-busy={currentModels?.loading}
                disabled={busy || currentModels?.loading}
                onClick={() => setModelRefresh((value) => value + 1)}
              >
                {currentModels?.loading
                  ? t("models.refreshing")
                  : t("models.refresh")}
              </Button>
            </FormActions>
          </FormSection>
        </ModalDialog>
      )}
      {deleting && (
        <ModalDialog
          open
          role="alertdialog"
          title={t("delete.title", {
            name: deleting.item.title || deleting.item.id,
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
                    "flame.schedules/delete.v2",
                    { id: deleting.item.id },
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
          page ? (
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
          ) : undefined
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
                rows.length
                  ? t("empty.filtered.title")
                  : paginated
                    ? t("empty.page.title")
                    : t("empty.none.title")
              }
              sub={rows.length || paginated ? undefined : t("empty.none.sub")}
              action={
                rows.length ? (
                  <Button
                    variant="outline"
                    onClick={() => {
                      filterPage("", "all");
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
            <ListRow key={item.id}>
              <RecordHeading
                title={item.title || t("untitled")}
                actions={
                  <ActionMenu
                    label={t("actions.more", {
                      name: item.title || t("untitled"),
                    })}
                    disabled={disabled}
                    finalFocus={
                      command?.command.method === "flame.schedules/runNow.v2"
                        ? feedbackElement
                        : undefined
                    }
                    items={[
                      {
                        label: t("action.runNow"),
                        onSelect: () =>
                          submit(
                            "flame.schedules/runNow.v2",
                            { id: item.id },
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
                              title: item.title,
                              instructions: item.instructions,
                              cron: item.cron,
                              timeZone: item.timeZone,
                              cwd: item.cwd,
                              modelSelection: structuredClone(
                                item.modelSelection,
                              ),
                              enabled: enabled(item),
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
                            "flame.schedules/update.v2",
                            {
                              id: item.id,
                              expectedRevision: item.revision,
                              enabled: !enabled(item),
                              modelSelection: {},
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
                // Enabled is the resting state; only a paused schedule needs a word.
                status={
                  enabled(item) ? undefined : (
                    <Badge>{t("status.disabled")}</Badge>
                  )
                }
              />
              <RecordBody>{item.instructions}</RecordBody>
              {/* A disabled schedule says so in its status; "not scheduled" would only repeat it. */}
              {(enabled(item) || item.nextRunAt !== undefined) && (
                <Metadata>
                  {t("nextFiring.label")} ·{" "}
                  {item.nextRunAt !== undefined ? (
                    <time dateTime={item.nextRunAt}>
                      {dateTime.format(new Date(item.nextRunAt))}
                    </time>
                  ) : (
                    t("nextFiring.none")
                  )}
                </Metadata>
              )}
              <Disclosure title={t("details.title")}>
                <Facts
                  values={[
                    [t("fact.cron"), item.cron],
                    [t("fact.timeZone"), item.timeZone],
                    [t("fact.schedule"), item.id],
                    [t("fact.revision"), String(item.revision)],
                    [t("fact.workspace"), item.cwd || t("runtimeDefault")],
                    [
                      t("field.model"),
                      item.modelSelection.provider && item.modelSelection.model
                        ? `${item.modelSelection.provider}/${item.modelSelection.model}`
                        : t("runtimeDefault"),
                    ],
                    [
                      t("field.reasoning"),
                      item.modelSelection.reasoningEffort || undefined,
                    ],
                    [
                      t("fact.lastFiring"),
                      item.lastRunAt !== undefined
                        ? dateTime.format(new Date(item.lastRunAt))
                        : t("fact.neverFired"),
                    ],
                    [
                      t("fact.created"),
                      dateTime.format(new Date(item.createdAt)),
                    ],
                  ]}
                />
              </Disclosure>
            </ListRow>
          ))}
        </RecordGroup>
        {paginated && (
          <PanelFooter>
            <Toolbar label={t("pages.label")}>
              <Button
                pending={read.loading && !busy}
                focusableWhenDisabled={!busy}
                disabled={busy || !history.length}
                onClick={(event) =>
                  void refresh(history.slice(0, -1), event.currentTarget)
                }
              >
                {t("pages.previous")}
              </Button>
              <Button
                pending={read.loading && !busy}
                focusableWhenDisabled={!busy}
                disabled={busy || !page?.nextCursor}
                onClick={(event) =>
                  void refresh(
                    [...history, page!.nextCursor],
                    event.currentTarget,
                  )
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

function ScheduleFeedback({
  t,
  notice,
  feedbackElement,
  command,
  reconcile,
  openSession,
}: Pick<
  ReturnType<typeof useSchedules>,
  "t" | "notice" | "feedbackElement" | "command" | "reconcile"
> & { openSession: ComponentContext["openSession"] }) {
  return (
    <section
      ref={feedbackElement}
      tabIndex={-1}
      data-focus-inset=""
      aria-label={t("command.status.label")}
    >
      {notice &&
        (typeof notice === "string" ? (
          <p role="status">{notice}</p>
        ) : (
          <>
            <p role="status">
              {t("notice.accepted", {
                run: notice.runId,
                session: notice.sessionId,
              })}
            </p>
            {openSession && (
              <Button
                variant="soft"
                onClick={() => openSession(notice.sessionId)}
              >
                {t("notice.openSession")}
              </Button>
            )}
          </>
        ))}
      {command && (
        <>
          <p role={command.phase === "uncertain" ? "alert" : "status"}>
            {command.phase === "uncertain"
              ? t("command.uncertain")
              : t("command.progress", { label: command.command.label })}
          </p>
          {command.phase === "uncertain" && (
            <>
              <RecordBody>{command.error}</RecordBody>
              <Metadata>
                {t("command.original", {
                  key: command.command.promise.idempotencyKey,
                })}
              </Metadata>
              <Button variant="soft" onClick={reconcile}>
                {t("command.reconcile")}
              </Button>
            </>
          )}
        </>
      )}
    </section>
  );
}

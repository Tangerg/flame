import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  CalendarClock,
  Refresh,
  Plus,
  Play,
  ChevronDown,
} from "@flame/icons/react";
import {
  bindOperations,
  mutationSettlementIsUnknown,
  OperationFailure,
  type ComponentContext,
} from "@flame/plugin-sdk";
import type { MutationPromise } from "@flame/runtime-contract/client";
import type { Model } from "@flame/runtime-contract/wire";
import {
  Badge,
  Button,
  Checkbox,
  EmptyState,
  Facts,
  ListRow,
  Metadata,
  Panel,
  PanelHeader,
  PanelSection,
  PanelFooter,
  FormRow,
  FormSection,
  FormActions,
  ActionMenu,
  IconButton,
  Icon,
  Disclosure,
  FormField,
  RecordForm,
  RecordBody,
  RecordGroup,
  RecordTitle,
  RecordHeading,
  SearchField,
  FilterBar,
  Select,
  TextArea,
  TextField,
  Toolbar,
} from "@flame/ui";
import {
  CAPABILITY_JSON,
  type MethodInputs,
  type MethodOutputs,
} from "../io.github.tangerg.flame/contracts/methods.generated";
import { templates } from "./templates";

type Schedule = MethodOutputs["flame.schedules/list.v1"]["Rows"][number];
type Page = MethodOutputs["flame.schedules/list.v1"];
type Draft = MethodInputs["flame.schedules/create.v1"];
type Mutation =
  | "flame.schedules/create.v1"
  | "flame.schedules/update.v1"
  | "flame.schedules/delete.v1"
  | "flame.schedules/runNow.v1";
type Command = {
  promise: MutationPromise<unknown>;
  label: string;
  uncertain: boolean;
  error?: string;
};
type Read = {
  loading: boolean;
  accepted?: { page: Page; history: string[] };
  error?: string;
};
type Edit = { basis?: Schedule; draft: Draft; opener: HTMLElement };
type ModelRead = {
  context: ComponentContext;
  catalogs: Map<string, Model[]>;
  loading: boolean;
  error?: string;
};
const hasTime = (value: string) => value !== "0001-01-01T00:00:00Z";
const enabled = (item: Schedule) => hasTime(item.NextRunAt);
const failure = (error: unknown) =>
  error instanceof Error
    ? error.message
    : "The operation could not be completed.";
const selection = (model?: Model, reasoningEffort = "") => ({
  provider: model?.provider ?? "",
  model: model?.id ?? "",
  reasoningEffort,
});

export function Schedules({ context }: { context: ComponentContext }) {
  const operations = useMemo(() => {
    if (!context.binding)
      throw new Error("Schedules requires its own callable contribution");
    return bindOperations<MethodInputs, MethodOutputs>(
      context,
      CAPABILITY_JSON,
      { scope: "installation", reference: context.binding.installationId },
    );
  }, [context]);
  const [read, setRead] = useState<Read>({ loading: true });
  const reading = useRef<AbortController>(undefined);
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("all");
  const [modelRead, setModelRead] = useState<ModelRead>();
  const [modelRefresh, setModelRefresh] = useState(0);
  const currentModels = modelRead?.context === context ? modelRead : undefined;
  const models = [...(currentModels?.catalogs.values() ?? [])].flat();
  const [edit, setEdit] = useState<Edit>();
  const [deleting, setDeleting] = useState<{
    item: Schedule;
    opener: HTMLElement;
  }>();
  const [command, setCommand] = useState<Command>();
  const pending = useRef<Command>(undefined);
  const accepted = useRef<{ page: Page; history: string[] }>(undefined);
  const [notice, setNotice] = useState<string>();
  const [liveError, setLiveError] = useState<string>();

  const refresh = useCallback(
    async (history = accepted.current?.history ?? []) => {
      reading.current?.abort();
      const controller = new AbortController();
      reading.current = controller;
      const signal = AbortSignal.any([context.signal, controller.signal]);
      setRead((previous) => ({ ...previous, loading: true, error: undefined }));
      try {
        const page = await operations.query(
          "flame.schedules/list.v1",
          { Cursor: history.at(-1) ?? "" },
          signal,
        );
        if (!signal.aborted) {
          accepted.current = { page, history };
          setRead({ loading: false, accepted: accepted.current });
        }
      } catch (error) {
        if (!signal.aborted)
          setRead((previous) => ({
            ...previous,
            loading: false,
            error: failure(error),
          }));
      }
    },
    [context, operations],
  );
  useEffect(() => {
    void refresh();
    return () => reading.current?.abort();
  }, [refresh]);
  useEffect(() => {
    const controller = new AbortController();
    const signal = AbortSignal.any([context.signal, controller.signal]);
    const watch = async () => {
      try {
        const subscription = await context.runtime.runtimeEvents.subscribe(
          { topics: ["plugins.dataChanged"] },
          signal,
        );
        if (signal.aborted) return;
        void refresh();
        for await (const event of subscription.events) {
          if (signal.aborted) break;
          if (event.type === "plugins.dataChanged" || event.type === "resync")
            void refresh();
        }
      } catch (error) {
        if (!signal.aborted) setLiveError(failure(error));
      }
    };
    void context.track(watch());
    return () => controller.abort();
  }, [context, refresh]);
  useEffect(() => {
    const controller = new AbortController();
    const signal = AbortSignal.any([context.signal, controller.signal]);
    setModelRead((previous) => ({
      context,
      catalogs: previous?.context === context ? previous.catalogs : new Map(),
      loading: true,
    }));
    const readModels = async () => {
      try {
        const page = await context.runtime.providers.list(signal);
        if (signal.aborted) return;
        const providers = page.data.filter((provider) => provider.configured);
        const results = await Promise.allSettled(
          providers.map((provider) =>
            context.runtime.models.list(provider.id, signal),
          ),
        );
        if (signal.aborted) return;
        setModelRead((previous) => {
          const catalogs = new Map<string, Model[]>();
          const errors: string[] = [];
          for (const [index, provider] of providers.entries()) {
            const result = results[index]!;
            if (result.status === "fulfilled")
              catalogs.set(provider.id, result.value.data);
            else {
              const retained =
                previous?.context === context
                  ? previous.catalogs.get(provider.id)
                  : undefined;
              if (retained) catalogs.set(provider.id, retained);
              errors.push(`${provider.id}: ${failure(result.reason)}`);
            }
          }
          return {
            context,
            catalogs,
            loading: false,
            error: errors.join("; ") || undefined,
          };
        });
      } catch (error) {
        if (!signal.aborted)
          setModelRead((previous) => ({
            context,
            catalogs:
              previous?.context === context ? previous.catalogs : new Map(),
            loading: false,
            error: failure(error),
          }));
      }
    };
    void context.track(readModels());
    return () => controller.abort();
  }, [context, modelRefresh]);

  async function observe(captured: Command) {
    try {
      const output = await context.track(captured.promise);
      pending.current = undefined;
      if (!context.signal.aborted) {
        setCommand(undefined);
        setEdit(undefined);
        setDeleting(undefined);
        setNotice(
          output &&
            typeof output === "object" &&
            "RunID" in output &&
            "SessionID" in output
            ? `Runtime accepted Run ${output.RunID} in Session ${output.SessionID}.`
            : `${captured.label} completed.`,
        );
        void refresh();
      }
    } catch (error) {
      if (context.signal.aborted) return;
      if (mutationSettlementIsUnknown(error)) {
        const uncertain = {
          ...captured,
          uncertain: true,
          error: failure(error),
        };
        pending.current = uncertain;
        setCommand(uncertain);
      } else {
        pending.current = undefined;
        setCommand(undefined);
        setNotice(
          error instanceof OperationFailure && error.code === "revision"
            ? "The schedule changed. Read its latest revision and open a new edit before saving."
            : failure(error),
        );
        if (error instanceof OperationFailure && error.code === "revision")
          setEdit(undefined);
        void refresh();
      }
    }
  }
  function submit<K extends Mutation>(
    method: K,
    input: MethodInputs[K],
    label: string,
  ) {
    if (pending.current || context.signal.aborted) return;
    try {
      const captured = {
        promise: operations.command(method, input),
        label,
        uncertain: false,
      };
      pending.current = captured;
      setCommand(captured);
      setNotice(undefined);
      void observe(captured);
    } catch (error) {
      if (!context.signal.aborted) setNotice(failure(error));
    }
  }
  function reconcile() {
    const original = pending.current;
    if (!original?.uncertain || context.signal.aborted) return;
    try {
      const retry = {
        ...original,
        promise: original.promise.retry({ signal: context.signal }),
        uncertain: false,
        error: undefined,
      };
      pending.current = retry;
      setCommand(retry);
      void observe(retry);
    } catch (error) {
      if (!context.signal.aborted)
        setCommand({ ...original, error: failure(error) });
    }
  }
  function add(opener: HTMLElement, template?: (typeof templates)[number]) {
    if (pending.current) return;
    setEdit({
      opener,
      draft: {
        Title: template?.title ?? "",
        Instructions: template?.instructions ?? "",
        Cron: template?.cron ?? "",
        CWD: context.workspace?.ref.path ?? "",
        ModelSelection: selection(),
        Enabled: true,
      },
    });
    setDeleting(undefined);
  }
  function patch<K extends keyof Draft>(key: K, value: Draft[K]) {
    setEdit((previous) =>
      previous
        ? { ...previous, draft: { ...previous.draft, [key]: value } }
        : undefined,
    );
  }
  function save() {
    if (!edit) return;
    const draft = {
      ...edit.draft,
      Title: edit.draft.Title.trim(),
      Instructions: edit.draft.Instructions.trim(),
      Cron: edit.draft.Cron.trim(),
      CWD: edit.draft.CWD.trim(),
    };
    if (edit.basis)
      submit(
        "flame.schedules/update.v1",
        {
          ID: edit.basis.ID,
          ExpectedRevision: edit.basis.Revision,
          Title: draft.Title,
          Instructions: draft.Instructions,
          Cron: draft.Cron,
          CWD: draft.CWD,
          Enabled: draft.Enabled,
          ModelSelection: {
            Provider: draft.ModelSelection.provider,
            Model: draft.ModelSelection.model,
            ReasoningEffort: draft.ModelSelection.reasoningEffort,
          },
        },
        "Save schedule",
      );
    else submit("flame.schedules/create.v1", draft, "Create schedule");
  }

  const page = read.accepted?.page;
  const rows = page?.Rows ?? [];
  const matching = rows.filter(
    (item) =>
      (status === "all" ||
        (enabled(item) ? "enabled" : "disabled") === status) &&
      `${item.ID} ${item.Title} ${item.Instructions} ${item.Cron} ${item.CWD}`
        .toLocaleLowerCase()
        .includes(search.toLocaleLowerCase()),
  );
  const busy = Boolean(command);
  const disabled = busy || read.loading || Boolean(read.error);
  const selectedModel = models.find(
    (model) =>
      model.provider === edit?.draft.ModelSelection.provider &&
      model.id === edit?.draft.ModelSelection.model,
  );
  const selectedKey =
    edit?.draft.ModelSelection.provider && edit.draft.ModelSelection.model
      ? JSON.stringify([
          edit.draft.ModelSelection.provider,
          edit.draft.ModelSelection.model,
        ])
      : "default";

  return (
    <Panel
      label="Schedules"
      header={
        <PanelHeader
          title="Schedules"
          description="Recurring work and the next time it will run."
        />
      }
      controls={
        <FilterBar
          label="Schedule controls"
          search={
            <SearchField
              size="lg"
              aria-label="Search this page"
              placeholder="Title or instructions"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
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
          <Toolbar label="Schedule actions">
            <IconButton
              icon={Refresh}
              aria-label="Refresh"
              size="md"
              disabled={read.loading || busy}
              onClick={() => void refresh()}
            />
            <Button
              variant={edit || deleting ? "outline" : "primary"}
              size="lg"
              disabled={busy || Boolean(edit) || Boolean(deleting)}
              onClick={(event) => add(event.currentTarget)}
            >
              <Icon glyph={Plus} size="sm" /> Add schedule
            </Button>
            <ActionMenu
              label="Schedule template"
              disabled={busy}
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
        </FilterBar>
      }
    >
      {read.loading && <p role="status">Reading schedules…</p>}
      {read.error && (
        <p role="alert">{read.error} The last accepted page is retained.</p>
      )}
      {liveError && (
        <p role="alert">
          Live refresh is unavailable: {liveError}. Use Refresh to read the
          current schedules.
        </p>
      )}
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
      {edit && (
        <RecordForm aria-label="Schedule editor" opener={edit.opener}>
          <RecordTitle>
            {edit.basis
              ? `Edit ${edit.basis.Title || "schedule"}`
              : "New schedule"}
          </RecordTitle>
          {edit.basis && (
            <Metadata>
              Captured revision: {String(edit.basis.Revision)}
            </Metadata>
          )}
          <FormSection
            title="Task"
            description="What the agent should do when this schedule runs."
          >
            <FormField label="Title">
              <TextField
                size="lg"
                aria-label="Title"
                // oxlint-disable-next-line jsx-a11y/no-autofocus
                autoFocus
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
          <FormActions label="Editor actions">
            <Button
              variant="ghost"
              disabled={busy}
              onClick={() => setEdit(undefined)}
            >
              Cancel
            </Button>
            <Button
              variant="primary"
              disabled={
                busy ||
                !edit.draft.Instructions.trim() ||
                !edit.draft.Cron.trim()
              }
              onClick={save}
            >
              Save schedule
            </Button>
          </FormActions>
        </RecordForm>
      )}
      {deleting && (
        <RecordForm
          aria-label="Confirm schedule deletion"
          opener={deleting.opener}
        >
          <RecordTitle>
            Delete {deleting.item.Title || deleting.item.ID}?
          </RecordTitle>
          <RecordBody>Captured work and accepted Runs are retained.</RecordBody>
          <FormActions label="Deletion confirmation">
            <Button
              variant="ghost"
              disabled={busy}
              // oxlint-disable-next-line jsx-a11y/no-autofocus
              autoFocus
              onClick={() => setDeleting(undefined)}
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
          </FormActions>
        </RecordForm>
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
        <RecordGroup variant="cards" id="items" aria-label="Saved schedules">
          {!matching.length && !read.loading && (
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
            <ListRow variant="card" key={item.ID}>
              <RecordHeading
                title={item.Title || "Untitled schedule"}
                actions={
                  <ActionMenu
                    label={`More actions for ${item.Title || "Untitled schedule"}`}
                    disabled={disabled}
                    items={[
                      {
                        label: "Edit",
                        onSelect: (opener) => {
                          setEdit({
                            opener,
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
                          setDeleting({ item: structuredClone(item), opener });
                          setEdit(undefined);
                        },
                      },
                    ]}
                  />
                }
                description={item.Instructions}
                status={
                  <Badge tone={enabled(item) ? "success" : "neutral"}>
                    {enabled(item) ? "Enabled" : "Disabled"}
                  </Badge>
                }
              />
              <Metadata>
                Cron · <code>{item.Cron}</code>
              </Metadata>
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
              <Disclosure
                title="Schedule details"
                actions={
                  <Button
                    variant="soft"
                    size="md"
                    disabled={disabled}
                    onClick={() =>
                      submit(
                        "flame.schedules/runNow.v1",
                        { ID: item.ID },
                        "Run schedule now",
                      )
                    }
                  >
                    <Icon glyph={Play} size="sm" /> Run now
                  </Button>
                }
              >
                <Facts
                  values={[
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
                      "Next firing",
                      hasTime(item.NextRunAt)
                        ? item.NextRunAt
                        : "Not scheduled",
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

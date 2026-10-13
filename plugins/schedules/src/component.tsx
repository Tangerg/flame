import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { CalendarClock } from "@flame/icons/react";
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
  PanelTitle,
  RecordBody,
  RecordGroup,
  RecordTitle,
  SearchField,
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
type Edit = { basis?: Schedule; draft: Draft };
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
  const [models, setModels] = useState<Model[]>([]);
  const [modelError, setModelError] = useState<string>();
  const [edit, setEdit] = useState<Edit>();
  const [deleting, setDeleting] = useState<Schedule>();
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
    const readModels = async () => {
      try {
        const page = await context.runtime.models.list(undefined, signal);
        if (!signal.aborted) setModels(page.data);
      } catch (error) {
        if (!signal.aborted) setModelError(failure(error));
      }
    };
    void context.track(readModels());
    return () => controller.abort();
  }, [context, refresh]);

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
  function add(template?: (typeof templates)[number]) {
    if (pending.current) return;
    setEdit({
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
    <Panel label="Schedules">
      <Toolbar label="Schedule actions">
        <PanelTitle>Schedules</PanelTitle>
        <Button disabled={busy} onClick={() => add()}>
          Add schedule
        </Button>
        <Select
          label="Schedule template"
          value="choose"
          disabled={busy}
          options={[
            { value: "choose", label: "Create from a template" },
            ...templates.map((template) => ({
              value: template.id,
              label: template.title,
            })),
          ]}
          onValueChange={(id) =>
            add(templates.find((template) => template.id === id))
          }
        />
        <Button disabled={read.loading || busy} onClick={() => void refresh()}>
          Refresh
        </Button>
      </Toolbar>
      <Metadata>
        Saved schedules. Editing or deleting a schedule leaves already captured
        work and accepted Runs unchanged.
      </Metadata>
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
        <section aria-label="Schedule editor">
          <RecordTitle>
            {edit.basis ? `Edit ${edit.basis.ID}` : "New schedule"}
          </RecordTitle>
          {edit.basis && (
            <Metadata>Captured revision: {edit.basis.Revision}</Metadata>
          )}
          <TextField
            aria-label="Title"
            placeholder="Title"
            value={edit.draft.Title}
            disabled={busy}
            onChange={(event) => patch("Title", event.target.value)}
          />
          <TextArea
            aria-label="Instructions"
            placeholder="Instructions"
            value={edit.draft.Instructions}
            disabled={busy}
            rows={4}
            onChange={(event) => patch("Instructions", event.target.value)}
          />
          <TextField
            aria-label="Cron expression"
            placeholder="Cron expression"
            value={edit.draft.Cron}
            disabled={busy}
            onChange={(event) => patch("Cron", event.target.value)}
          />
          <TextField
            aria-label="Workspace path"
            placeholder="Empty uses the Runtime default"
            value={edit.draft.CWD}
            disabled={busy}
            onChange={(event) => patch("CWD", event.target.value)}
          />
          <Select
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
              ...(selectedKey !== "default" && !selectedModel
                ? [
                    {
                      value: selectedKey,
                      label: `${edit.draft.ModelSelection.provider} / ${edit.draft.ModelSelection.model} (unavailable)`,
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
          <Select
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
          {modelError && (
            <p role="alert">Model choices could not be read: {modelError}</p>
          )}
          <Checkbox
            label="Enabled"
            checked={edit.draft.Enabled}
            disabled={busy}
            onCheckedChange={(value) => patch("Enabled", value)}
          />
          <Toolbar label="Editor actions">
            <Button
              disabled={
                busy ||
                !edit.draft.Instructions.trim() ||
                !edit.draft.Cron.trim()
              }
              onClick={save}
            >
              Save schedule
            </Button>
            <Button disabled={busy} onClick={() => setEdit(undefined)}>
              Cancel
            </Button>
          </Toolbar>
        </section>
      )}
      {deleting && (
        <section aria-label="Confirm schedule deletion">
          <RecordTitle>Delete {deleting.Title || deleting.ID}?</RecordTitle>
          <RecordBody>Captured work and accepted Runs are retained.</RecordBody>
          <Toolbar label="Deletion confirmation">
            <Button
              disabled={busy}
              onClick={() =>
                submit(
                  "flame.schedules/delete.v1",
                  { ID: deleting.ID },
                  "Delete schedule",
                )
              }
            >
              Confirm deletion
            </Button>
            <Button disabled={busy} onClick={() => setDeleting(undefined)}>
              Cancel
            </Button>
          </Toolbar>
        </section>
      )}
      <Toolbar label="Schedule filters">
        <SearchField
          aria-label="Search this page"
          placeholder="Title or instructions"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
        <Select
          label="Schedule status"
          value={status}
          onValueChange={setStatus}
          options={[
            { value: "all", label: "All statuses" },
            { value: "enabled", label: "Enabled" },
            { value: "disabled", label: "Disabled" },
          ]}
        />
      </Toolbar>
      <Metadata>
        {rows.filter(enabled).length} enabled ·{" "}
        {rows.filter((item) => !enabled(item)).length} disabled on this page
      </Metadata>
      <RecordGroup id="items" aria-label="Saved schedules">
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
          <ListRow key={item.ID}>
            <Toolbar label={item.Title || "Untitled schedule"}>
              <RecordTitle>{item.Title || "Untitled schedule"}</RecordTitle>
              <Badge tone={enabled(item) ? "success" : "neutral"}>
                {enabled(item) ? "Enabled" : "Disabled"}
              </Badge>
            </Toolbar>
            <RecordBody>{item.Instructions}</RecordBody>
            <Facts
              values={[
                ["Schedule", item.ID],
                ["Revision", item.Revision],
                ["Cron", item.Cron],
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
                  hasTime(item.NextRunAt) ? item.NextRunAt : "Not scheduled",
                ],
                [
                  "Last firing",
                  hasTime(item.LastRunAt) ? item.LastRunAt : "Never fired",
                ],
                ["Created", item.CreatedAt],
              ]}
            />
            <Toolbar label={`Actions for ${item.ID}`}>
              <Button
                disabled={disabled}
                onClick={() => {
                  setEdit({
                    basis: structuredClone(item),
                    draft: {
                      Title: item.Title,
                      Instructions: item.Instructions,
                      Cron: item.Cron,
                      CWD: item.CWD,
                      ModelSelection: structuredClone(item.ModelSelection),
                      Enabled: enabled(item),
                    },
                  });
                  setDeleting(undefined);
                }}
              >
                Edit
              </Button>
              <Button
                disabled={disabled}
                onClick={() =>
                  submit(
                    "flame.schedules/update.v1",
                    {
                      ID: item.ID,
                      ExpectedRevision: item.Revision,
                      Enabled: !enabled(item),
                      ModelSelection: {},
                    },
                    enabled(item) ? "Disable schedule" : "Enable schedule",
                  )
                }
              >
                {enabled(item) ? "Disable" : "Enable"}
              </Button>
              <Button
                disabled={disabled}
                onClick={() =>
                  submit(
                    "flame.schedules/runNow.v1",
                    { ID: item.ID },
                    "Run schedule now",
                  )
                }
              >
                Run now
              </Button>
              <Button
                disabled={disabled}
                onClick={() => {
                  setDeleting(structuredClone(item));
                  setEdit(undefined);
                }}
              >
                Delete
              </Button>
            </Toolbar>
          </ListRow>
        ))}
      </RecordGroup>
      <Toolbar label="Schedule pages">
        <Button
          disabled={read.loading || busy || !read.accepted?.history.length}
          onClick={() => void refresh(read.accepted!.history.slice(0, -1))}
        >
          Previous
        </Button>
        <Button
          disabled={read.loading || busy || !page?.NextCursor}
          onClick={() =>
            void refresh([...(read.accepted?.history ?? []), page!.NextCursor])
          }
        >
          Next
        </Button>
      </Toolbar>
    </Panel>
  );
}

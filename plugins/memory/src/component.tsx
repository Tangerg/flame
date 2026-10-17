import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  bindOperations,
  mutationSettlementIsUnknown,
  type ComponentContext,
} from "@flame/plugin-sdk";
import type { MutationPromise } from "@flame/runtime-contract/client";
import {
  CAPABILITY_JSON,
  type MethodInputs,
  type MethodOutputs,
} from "../io.github.tangerg.flame/contracts/methods.generated";
import { Brain, Refresh, Plus } from "@flame/icons/react";

import {
  Button,
  Panel,
  PanelHeader,
  IconButton,
  Icon,
  Segmented,
  PanelSection,
  PanelFooter,
  ActionMenu,
  Disclosure,
  Facts,
  FormField,
  FormActions,
  RecordForm,
  RecordTitle,
  TextArea,
  Badge,
  Checkbox,
  EmptyState,
  ListRow,
  Metadata,
  RecordBody,
  RecordMeta,
  RecordGroup,
  SearchField,
  FilterBar,
  Select,
  Toolbar,
} from "@flame/ui";

type MemoryItem = MethodOutputs["flame.memory/list.v1"]["Rows"][number];
type MemoryPage = MethodOutputs["flame.memory/list.v1"];

function MemoryReport({
  page,
  disabled,
  authoring,
  edit,
  remove,
  command,
  search,
  status,
  origin,
  pinned,
}: {
  page: MemoryPage;
  disabled: boolean;
  authoring: boolean;
  edit(item: MemoryItem, opener: HTMLElement): void;
  remove(item: MemoryItem, opener: HTMLElement): void;
  command<K extends MutationName>(method: K, input: MethodInputs[K]): void;
  search: string;
  status: string;
  origin: string;
  pinned: boolean;
}) {
  const rows = page.Rows;
  const matching = rows.filter(
    (item) =>
      (status === "all" || item.Status === status) &&
      (origin === "all" || item.Origin === origin) &&
      (!pinned || item.Pinned) &&
      `${item.ID} ${item.Content} ${item.SessionID ?? ""}`
        .toLocaleLowerCase()
        .includes(search.toLocaleLowerCase()),
  );
  return (
    <>
      <PanelSection
        title="Saved memory"
        description={
          <>
            <span id="summary">{`${rows.filter((item) => item.Status === "pending").length} pending · ${rows.filter((item) => item.Status === "active").length} active on this page`}</span>
          </>
        }
      >
        <RecordGroup variant="list" id="items" aria-label="Recorded memory">
          {!matching.length && (
            <EmptyState
              icon={Brain}
              size="compact"
              title={
                rows.length
                  ? "No matching items on this page."
                  : "No memory items in this target."
              }
              sub={
                rows.length
                  ? "Clear the search and filters."
                  : "Use Add memory above to save a fact."
              }
            />
          )}
          {matching.map((item) => (
            <ListRow
              key={item.ID}
              data-memory-id={item.ID}
              data-status={item.Status}
            >
              <RecordBody>{item.Content}</RecordBody>
              <Disclosure
                title={
                  <RecordMeta>
                    <span>Record details</span>
                    <Badge
                      tone={item.Status === "pending" ? "warning" : "neutral"}
                    >
                      {item.Status === "pending" ? "Pending review" : "Active"}
                    </Badge>
                    {item.Pinned && <Badge>Pinned</Badge>}
                  </RecordMeta>
                }
                actions={
                  <Toolbar label={`Actions for ${item.ID}`}>
                    {item.Status === "pending" ? (
                      <>
                        <Button
                          variant="soft"
                          size="md"
                          disabled={disabled}
                          onClick={() =>
                            command("flame.memory/review.v1", {
                              ID: item.ID,
                              Decision: "approve",
                            })
                          }
                        >
                          Approve
                        </Button>
                        <Button
                          variant="wash"
                          tone="negative"
                          size="md"
                          disabled={disabled}
                          onClick={() =>
                            command("flame.memory/review.v1", {
                              ID: item.ID,
                              Decision: "reject",
                            })
                          }
                        >
                          Reject
                        </Button>
                      </>
                    ) : (
                      <>
                        <Button
                          variant="soft"
                          size="md"
                          disabled={disabled || authoring}
                          onClick={(event) => edit(item, event.currentTarget)}
                        >
                          Edit
                        </Button>
                        <ActionMenu
                          label={`More actions for ${item.ID}`}
                          disabled={disabled}
                          items={[
                            {
                              label: item.Pinned ? "Unpin" : "Pin",
                              onSelect: () =>
                                command("flame.memory/update.v1", {
                                  ID: item.ID,
                                  Pinned: !item.Pinned,
                                }),
                            },
                            {
                              label: "Delete",
                              tone: "negative",
                              disabled: authoring,
                              onSelect: (opener) => remove(item, opener),
                            },
                          ]}
                        />
                      </>
                    )}
                  </Toolbar>
                }
              >
                <Facts
                  values={[
                    ["ID", item.ID],
                    ["Scope", item.Scope],
                    [
                      "Origin",
                      item.Origin === "auto" ? "Mined" : "User authored",
                    ],
                    ["Source Session", item.SessionID],
                    ["Day", item.Day],
                  ]}
                />
              </Disclosure>
            </ListRow>
          ))}
        </RecordGroup>
      </PanelSection>
    </>
  );
}
type MutationName =
  | "flame.memory/add.v1"
  | "flame.memory/update.v1"
  | "flame.memory/delete.v1"
  | "flame.memory/review.v1";
type Scope = "project" | "user";
type Accepted = {
  page: MemoryPage;
  cursors: string[];
  scope: Scope;
  cwd: string;
};
type Read = { loading: boolean; accepted?: Accepted; error?: string };
type Edit = (
  | { type: "add"; content: string }
  | { type: "update"; item: MemoryItem; content: string }
  | { type: "delete"; item: MemoryItem }
) & {
  opener: HTMLElement;
};
type Command = {
  promise: MutationPromise<unknown>;
  // Updates also pin independent records, so the method cannot identify the submitting form.
  source: "editor" | "record";
};

export function Memory({ context }: { context: ComponentContext }) {
  const operations = useMemo(() => {
    if (!context.binding)
      throw new Error("Memory requires its declared callable contribution");
    return bindOperations<MethodInputs, MethodOutputs>(
      context,
      CAPABILITY_JSON,
      {
        scope: "installation",
        reference: context.binding.installationId,
      },
    );
  }, [context]);
  const [scope, setScope] = useState<Scope>("project");
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("all");
  const [origin, setOrigin] = useState("all");
  const [pinned, setPinned] = useState(false);
  const cwd = scope === "project" ? context.workspace?.ref.path : "";
  const [read, setRead] = useState<Read>({ loading: false });
  const [edit, setEdit] = useState<Edit>();
  const [pending, setPending] = useState(false);
  const [unknown, setUnknown] = useState<Command>();
  const [message, setMessage] = useState<string>();
  const reading = useRef<AbortController>(undefined);
  const lifetime = useRef<AbortController>(undefined);
  const command = useRef<Command>(undefined);
  const observing = useRef(false);

  const load = useCallback(
    async (owner: AbortSignal, cursors = [""]) => {
      reading.current?.abort();
      if (cwd === undefined) {
        setRead((previous) => ({
          ...previous,
          loading: false,
          error: undefined,
        }));
        return;
      }
      const observer = new AbortController();
      reading.current = observer;
      const signal = AbortSignal.any([context.signal, owner, observer.signal]);
      if (signal.aborted) return;
      setRead((previous) => ({ ...previous, loading: true, error: undefined }));
      try {
        const page = await operations.query(
          "flame.memory/list.v1",
          {
            Scope: scope,
            CWD: cwd,
            Cursor: cursors.at(-1)!,
          },
          signal,
        );
        if (!signal.aborted)
          setRead({ loading: false, accepted: { page, cursors, scope, cwd } });
      } catch (error) {
        if (!signal.aborted)
          setRead((previous) => ({
            ...previous,
            loading: false,
            error:
              error instanceof Error
                ? error.message
                : "Memory could not be read.",
          }));
      }
    },
    [context, operations, scope, cwd],
  );
  useEffect(() => {
    const owner = new AbortController();
    lifetime.current = owner;
    const signal = AbortSignal.any([context.signal, owner.signal]);
    void load(signal);
    return () => owner.abort();
  }, [context.signal, load]);
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
        void load(signal);
        for await (const event of subscription.events) {
          if (signal.aborted) break;
          if (event.type === "plugins.dataChanged" || event.type === "resync")
            void load(signal);
        }
      } catch (error) {
        if (!signal.aborted)
          setRead((previous) => ({
            ...previous,
            error:
              error instanceof Error
                ? error.message
                : "Memory updates could not be observed.",
          }));
      }
    };
    void context.track(watch());
    return () => controller.abort();
  }, [context, load]);
  const current =
    read.accepted?.scope === scope && read.accepted.cwd === cwd
      ? read.accepted
      : undefined;
  const locked = pending || Boolean(unknown) || context.signal.aborted;
  function observerSignal(): AbortSignal | undefined {
    const owner = lifetime.current;
    if (!owner || owner.signal.aborted || context.signal.aborted) return;
    return AbortSignal.any([context.signal, owner.signal]);
  }
  async function observe(original: Command, owner: AbortSignal) {
    if (observing.current) return;
    observing.current = true;
    setPending(true);
    setMessage(undefined);
    try {
      await original.promise;
      if (owner.aborted) return;
      command.current = undefined;
      setUnknown(undefined);
      if (original.source === "editor") setEdit(undefined);
      setMessage("Memory change saved.");
      await load(owner);
    } catch (error) {
      if (owner.aborted) return;
      if (mutationSettlementIsUnknown(error)) {
        setUnknown(() => original);
        setMessage(
          "Confirmation is unknown. Check this original command before submitting another change.",
        );
      } else {
        command.current = undefined;
        setUnknown(undefined);
        setMessage(
          error instanceof Error ? error.message : "The change was refused.",
        );
      }
    } finally {
      observing.current = false;
      if (!owner.aborted) setPending(false);
    }
  }
  function change<K extends MutationName>(
    method: K,
    input: MethodInputs[K],
    source: Command["source"] = "record",
  ) {
    const owner = observerSignal();
    if (!owner || command.current || observing.current || cwd === undefined)
      return;
    try {
      const original = { promise: operations.command(method, input), source };
      command.current = original;
      void observe(original, owner);
    } catch (error) {
      setMessage(
        error instanceof Error
          ? error.message
          : "The change could not be submitted.",
      );
    }
  }
  function reconcile() {
    const owner = observerSignal();
    if (!owner || !unknown || observing.current) return;
    try {
      const attempt = {
        ...unknown,
        promise: unknown.promise.retry({ signal: owner }),
      };
      command.current = attempt;
      void observe(attempt, owner);
    } catch (error) {
      setMessage(
        error instanceof Error
          ? error.message
          : "The original result could not be checked.",
      );
    }
  }
  return (
    <Panel
      label="Agent memory"
      header={
        <PanelHeader
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
          description="Facts and preferences the agent can use in future work."
        />
      }
      controls={
        <>
          <Toolbar label="Memory navigation">
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
                setStatus("all");
                setOrigin("all");
                setPinned(false);
                setScope(value as Scope);
              }}
              disabled={locked || Boolean(edit)}
            />
          </Toolbar>
          <FilterBar
            label="Memory controls"
            search={
              <SearchField
                size="lg"
                id="search"
                aria-label="Search this page"
                placeholder="Recorded text or ID"
                value={search}
                onChange={(event) => setSearch(event.target.value)}
              />
            }
          >
            <Select
              size="lg"
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
              size="lg"
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
            <Toolbar label="Memory actions">
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
                size="lg"
                disabled={locked || cwd === undefined || Boolean(edit)}
                onClick={(event) => {
                  setEdit({
                    type: "add",
                    content: "",
                    opener: event.currentTarget,
                  });
                }}
              >
                <Icon glyph={Plus} size="sm" /> Add memory
              </Button>
            </Toolbar>
          </FilterBar>
        </>
      }
    >
      {read.loading && <p role="status">Reading recorded memory…</p>}
      {read.error && <p role="alert">{read.error}</p>}
      {message && <p role="status">{message}</p>}
      {unknown && (
        <Toolbar label="Original Memory command">
          <Metadata>
            Original command: {unknown.promise.idempotencyKey}
          </Metadata>
          <Button disabled={pending} onClick={reconcile}>
            Check original result
          </Button>
        </Toolbar>
      )}
      {edit?.type === "delete" ? (
        <RecordForm aria-label="Confirm memory deletion" opener={edit.opener}>
          <RecordTitle>Delete this memory?</RecordTitle>
          <RecordBody>{edit.item.Content}</RecordBody>
          <Metadata>
            This memory will no longer be available to the agent.
          </Metadata>
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
                change("flame.memory/delete.v1", { ID: edit.item.ID }, "editor")
              }
            >
              Delete memory
            </Button>
          </FormActions>
        </RecordForm>
      ) : edit ? (
        <RecordForm
          aria-label={edit.type === "add" ? "Add memory" : "Edit memory"}
          opener={edit.opener}
        >
          <RecordTitle>
            {edit.type === "add" ? "Add memory" : "Edit memory"}
          </RecordTitle>
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
                      "editor",
                    )
                  : change(
                      "flame.memory/update.v1",
                      {
                        ID: edit.item.ID,
                        Content: edit.content.trim(),
                      },
                      "editor",
                    )
              }
            >
              Save
            </Button>
          </FormActions>
        </RecordForm>
      ) : null}
      {current && (
        <>
          <MemoryReport
            page={current.page}
            search={search}
            status={status}
            origin={origin}
            pinned={pinned}
            authoring={Boolean(edit)}
            disabled={locked || read.loading || Boolean(read.error)}
            edit={(item, opener) => {
              setEdit({
                type: "update",
                item: structuredClone(item),
                content: item.Content,
                opener,
              });
            }}
            remove={(item, opener) => {
              setEdit({ type: "delete", item: structuredClone(item), opener });
            }}
            command={change}
          />
          {(current.cursors.length > 1 || Boolean(current.page.NextCursor)) && (
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
                  disabled={read.loading || pending || !current.page.NextCursor}
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

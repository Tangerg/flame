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
import { Brain } from "@flame/icons/react";

import {
  Button,
  Panel,
  PanelTitle,
  TextArea,
  Badge,
  Checkbox,
  EmptyState,
  ListRow,
  Metadata,
  RecordBody,
  RecordGroup,
  SearchField,
  Select,
  Toolbar,
} from "@flame/ui";

type MemoryItem = MethodOutputs["flame.memory/list.v1"]["Rows"][number];
type MemoryPage = MethodOutputs["flame.memory/list.v1"];

export function MemoryReport({
  page,
  disabled,
  edit,
  command,
}: {
  page: MemoryPage;
  disabled: boolean;
  edit(item: MemoryItem): void;
  command<K extends MutationName>(method: K, input: MethodInputs[K]): void;
}) {
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("all");
  const [origin, setOrigin] = useState("all");
  const [pinned, setPinned] = useState(false);
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
      <Toolbar label="Memory filters">
        <SearchField
          id="search"
          aria-label="Search this page"
          placeholder="Recorded text or ID"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
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
      <Metadata>
        <span id="summary">{`${rows.filter((item) => item.Status === "pending").length} pending · ${rows.filter((item) => item.Status === "active").length} active on this page`}</span>
      </Metadata>
      <RecordGroup id="items" aria-label="Recorded memory">
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
            <Badge tone={item.Status === "pending" ? "accent" : "neutral"}>
              {item.Status === "pending" ? "Pending review" : "Active"}
            </Badge>
            <RecordBody>{item.Content}</RecordBody>
            <Metadata>
              {item.Scope} ·{" "}
              {item.Origin === "auto" ? "Mined" : "User authored"}
              {item.Pinned ? " · Pinned" : ""}
            </Metadata>
            <Metadata>ID: {item.ID}</Metadata>
            {item.SessionID && (
              <Metadata>Source Session: {item.SessionID}</Metadata>
            )}
            {item.Day && <Metadata>Day: {item.Day}</Metadata>}
            <Toolbar label={`Actions for ${item.ID}`}>
              {item.Status === "pending" ? (
                <>
                  <Button
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
                  <Button disabled={disabled} onClick={() => edit(item)}>
                    Edit
                  </Button>
                  <Button
                    disabled={disabled}
                    onClick={() =>
                      command("flame.memory/update.v1", {
                        ID: item.ID,
                        Pinned: !item.Pinned,
                      })
                    }
                  >
                    {item.Pinned ? "Unpin" : "Pin"}
                  </Button>
                  <Button
                    disabled={disabled}
                    onClick={() =>
                      command("flame.memory/delete.v1", { ID: item.ID })
                    }
                  >
                    Delete
                  </Button>
                </>
              )}
            </Toolbar>
          </ListRow>
        ))}
      </RecordGroup>
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
type Edit = { type: "add" } | { type: "update"; item: MemoryItem };

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
  const cwd = scope === "project" ? context.workspace?.ref.path : "";
  const [read, setRead] = useState<Read>({ loading: false });
  const [edit, setEdit] = useState<Edit>();
  const [draft, setDraft] = useState("");
  const [pending, setPending] = useState(false);
  const [unknown, setUnknown] = useState<MutationPromise<unknown>>();
  const [message, setMessage] = useState<string>();
  const reading = useRef<AbortController>(undefined);
  const lifetime = useRef<AbortController>(undefined);
  const command = useRef<MutationPromise<unknown>>(undefined);
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
            error: error instanceof Error ? error.message : "Memory updates could not be observed.",
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
  async function observe(
    original: MutationPromise<unknown>,
    owner: AbortSignal,
  ) {
    if (observing.current) return;
    observing.current = true;
    setPending(true);
    setMessage(undefined);
    try {
      await original;
      if (owner.aborted) return;
      command.current = undefined;
      setUnknown(undefined);
      setEdit(undefined);
      setDraft("");
      setMessage("Change confirmed by the Memory owner.");
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
  function change<K extends MutationName>(method: K, input: MethodInputs[K]) {
    const owner = observerSignal();
    if (!owner || command.current || observing.current || cwd === undefined)
      return;
    try {
      const original = operations.command(method, input);
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
      const attempt = unknown.retry({ signal: owner });
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
    <Panel label="Agent memory">
      <Toolbar label="Memory target and actions">
        <PanelTitle>Agent memory</PanelTitle>
        <Select
          label="Memory scope"
          value={scope}
          options={[
            { value: "project", label: "Current project" },
            { value: "user", label: "User" },
          ]}
          onValueChange={(value) => {
            if (command.current || observing.current) return;
            setEdit(undefined);
            setDraft("");
            setScope(value as Scope);
          }}
          disabled={locked}
        />
        <Button
          disabled={read.loading || cwd === undefined || pending}
          onClick={() => {
            const owner = observerSignal();
            if (owner) void load(owner);
          }}
        >
          Refresh
        </Button>
        <Button
          disabled={locked || cwd === undefined}
          onClick={() => {
            setEdit({ type: "add" });
            setDraft("");
          }}
        >
          Add memory
        </Button>
      </Toolbar>
      <Metadata>
        {scope === "user"
          ? "User memory"
          : cwd
            ? `Project memory · ${cwd}`
            : "Choose a Session with a workspace, or select User memory."}
      </Metadata>
      {read.loading && <p role="status">Reading recorded memory…</p>}
      {read.error && <p role="alert">{read.error}</p>}
      {message && <p role="status">{message}</p>}
      {unknown && (
        <Toolbar label="Original Memory command">
          <Metadata>Original command: {unknown.idempotencyKey}</Metadata>
          <Button disabled={pending} onClick={reconcile}>
            Check original result
          </Button>
        </Toolbar>
      )}
      {edit && (
        <section
          aria-label={edit.type === "add" ? "Add memory" : "Edit memory"}
        >
          <TextArea
            aria-label="Memory content"
            rows={3}
            value={draft}
            pending={locked}
            onChange={(event) => setDraft(event.target.value)}
          />
          <Toolbar label="Save memory content">
            <Button
              disabled={
                locked ||
                !draft.trim() ||
                (edit.type === "update" && draft.trim() === edit.item.Content)
              }
              onClick={() =>
                edit.type === "add"
                  ? change("flame.memory/add.v1", {
                      Scope: scope,
                      CWD: cwd!,
                      Content: draft.trim(),
                    })
                  : change("flame.memory/update.v1", {
                      ID: edit.item.ID,
                      Content: draft.trim(),
                    })
              }
            >
              Save
            </Button>
            <Button
              disabled={locked}
              variant="ghost"
              onClick={() => setEdit(undefined)}
            >
              Cancel
            </Button>
          </Toolbar>
        </section>
      )}
      {current && (
        <>
          <Metadata>
            Last accepted page {current.cursors.length};{" "}
            {current.page.Rows.length} recorded items.
          </Metadata>
          <MemoryReport
            page={current.page}
            disabled={locked || read.loading || Boolean(read.error)}
            edit={(item) => {
              setEdit({ type: "update", item });
              setDraft(item.Content);
            }}
            command={change}
          />
          <Toolbar label="Memory pages">
            <Button
              disabled={read.loading || pending || current.cursors.length === 1}
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
          </Toolbar>
        </>
      )}
    </Panel>
  );
}

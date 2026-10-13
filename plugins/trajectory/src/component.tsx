import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  mutationSettlementIsUnknown,
  type ComponentContext,
} from "@flame/plugin-sdk";
import type { MutationPromise } from "@flame/runtime-contract/client";
import type { ReactNode } from "react";
import { Activity } from "@flame/icons/react";
import type {
  Page,
  Session,
  TrajectoryEntry,
} from "@flame/runtime-contract/wire";
import {
  Button,
  Panel,
  PanelTitle,
  CodeBlock,
  Disclosure,
  EmptyState,
  EvidenceImage,
  Facts,
  Metadata,
  RecordGroup,
  SearchField,
  Select,
  Stats,
  Toolbar,
  TextField,
  type Fact,
} from "@flame/ui";

const runId = (row: TrajectoryEntry) =>
  row.type === "run"
    ? row.run.id
    : row.type === "model"
      ? row.model.runId
      : row.item.runId;
function needsAttention(row: TrajectoryEntry): boolean {
  if (row.type === "model")
    return row.model.state === "failed" || row.model.state === "unknown";
  if (row.type === "run")
    return (
      row.run.status === "waiting" ||
      ["failed", "lost"].includes(row.run.outcome?.type ?? "")
    );
  return (
    row.item.status === "incomplete" ||
    (row.item.type === "toolCall" &&
      (row.item.approvalDecision === "deny" || Boolean(row.item.error)))
  );
}
function evidence(row: TrajectoryEntry): TrajectoryEntry {
  if (
    row.type !== "item" ||
    row.item.type !== "reasoning" ||
    !row.item.redacted
  )
    return row;
  const { text: _redacted, ...item } = row.item;
  return { ...row, item };
}
function title(row: TrajectoryEntry): string {
  if (row.type === "run")
    return `Run · ${row.run.id} · ${row.run.provider}/${row.run.model} · ${row.run.outcome?.type ?? row.run.status}`;
  if (row.type === "model")
    return `Model call · ${row.model.callId} · ${row.model.state}`;
  const item = row.item;
  return `${item.type} · ${item.id} · ${item.type === "toolCall" ? item.tool.name : item.status}${item.type === "toolCall" && item.approvalDecision ? ` · ${item.approvalDecision}` : ""}`;
}
function Observation({ row }: { row: TrajectoryEntry }) {
  const facts: Fact[] = [["Run ID", runId(row)]];
  const notes: ReactNode[] = [];
  if (row.type === "model") {
    const source = row.model;
    facts.push(
      ["Segment ID", source.segmentId],
      ["Recorded settlement", source.settledAt ?? "Unrecorded"],
    );
    if (["completed", "failed"].includes(source.state) && source.settledAt) {
      const duration =
        Date.parse(source.settledAt) - Date.parse(source.startedAt);
      if (Number.isFinite(duration) && duration >= 0)
        facts.push(["Wall duration (ms)", duration]);
    }
    facts.push(
      ["First output (ms)", source.firstOutputLatencyMillis ?? "Unrecorded"],
      ["Input tokens", source.usage?.inputTokens ?? "Unrecorded"],
      ["Output tokens", source.usage?.outputTokens ?? "Unrecorded"],
    );
    for (const [key, label] of [
      ["cacheReadTokens", "Cache read tokens"],
      ["cacheWriteTokens", "Cache write tokens"],
      ["reasoningTokens", "Reasoning tokens"],
    ] as const)
      if (source.usage?.[key] !== undefined)
        facts.push([label, source.usage[key]]);
    if (!source.usage) notes.push("Usage was not recorded for this call.");
    if (source.state === "unknown")
      notes.push(
        "The final outcome was not observed. Settlement time is not measured execution time.",
      );
  } else if (row.type === "run") {
    facts.push(["Root Run ID", row.run.rootRunId ?? "Unrecorded"]);
    if (row.run.parentRunId) facts.push(["Parent Run ID", row.run.parentRunId]);
    if (row.run.spawnedByItemId)
      facts.push(["Delegating Item ID", row.run.spawnedByItemId]);
  } else if (row.item.type === "toolCall") {
    facts.push([
      "Execution duration (ms)",
      row.item.durationMillis ?? "Unrecorded",
    ]);
    if (row.item.approvalDecision)
      facts.push(["Approval", row.item.approvalDecision]);
  }
  if (row.type === "item" && row.item.type === "reasoning" && row.item.redacted)
    notes.push("Reasoning was redacted.");
  return (
    <>
      <Facts values={facts} />
      {notes.map((note, index) => (
        <Metadata key={index}>{note}</Metadata>
      ))}
      <CodeBlock label="Recorded evidence">
        {JSON.stringify(evidence(row), null, 2)}
      </CodeBlock>
      {row.type === "item" &&
        "content" in row.item &&
        row.item.content?.map((part, index) =>
          part.type === "image" ? (
            <EvidenceImage
              key={index}
              src={`data:${part.mime};base64,${part.data}`}
              alt={`Recorded ${part.mime} image`}
            />
          ) : null,
        )}
    </>
  );
}
export function TrajectoryReport({ page }: { page: Page<TrajectoryEntry> }) {
  const [kind, setKind] = useState("all");
  const [run, setRun] = useState("all");
  const [search, setSearch] = useState("");
  const rows = page.data;
  const runOptions = [
    { value: "all", label: "All Runs" },
    ...Array.from(new Set(rows.map(runId))).map((id) => ({
      value: id,
      label: id,
    })),
  ];
  const selectedRun = runOptions.some((option) => option.value === run)
    ? run
    : "all";
  const matching = rows.filter((row) => {
    const category = row.type === "item" ? row.item.type : row.type;
    const matchesKind =
      kind === "all" ||
      kind === category ||
      (kind === "attention" && needsAttention(row)) ||
      (kind === "message" &&
        ["userMessage", "agentMessage", "reasoning"].includes(category));
    return (
      matchesKind &&
      (selectedRun === "all" || runId(row) === selectedRun) &&
      JSON.stringify(evidence(row), (key, value) =>
        key === "data" ? undefined : value,
      )
        .toLocaleLowerCase()
        .includes(search.toLocaleLowerCase())
    );
  });
  return (
    <>
      <Stats
        id="metrics"
        aria-label="Current page observations"
        values={[
          ["Page records", rows.length],
          ["Model calls", rows.filter((row) => row.type === "model").length],
          [
            "Tool calls",
            rows.filter(
              (row) => row.type === "item" && row.item.type === "toolCall",
            ).length,
          ],
          ["Needs attention", rows.filter(needsAttention).length],
        ]}
      />
      <Toolbar label="Observation filters">
        <Select
          id="kind"
          label="Observation type"
          value={kind}
          onValueChange={setKind}
          options={[
            { value: "all", label: "All observations" },
            { value: "run", label: "Runs" },
            { value: "model", label: "Model calls" },
            { value: "toolCall", label: "Tool calls" },
            { value: "message", label: "Messages" },
            { value: "attention", label: "Needs attention" },
          ]}
        />
        <Select
          id="run"
          label="Run filter"
          value={selectedRun}
          onValueChange={setRun}
          options={runOptions}
        />
        <SearchField
          id="search"
          aria-label="Filter this page"
          placeholder="Recorded text or ID"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
      </Toolbar>
      <RecordGroup id="records" aria-label="Trajectory observations">
        {matching.map((row) => (
          <Disclosure
            key={`${row.type}:${row.type === "model" ? row.model.callId : row.type === "run" ? row.run.id : row.item.id}`}
            data-trajectory-record={`${row.type}:${row.type === "model" ? row.model.callId : row.type === "run" ? row.run.id : row.item.id}`}
            data-trajectory-kind={
              row.type === "item" ? row.item.type : row.type
            }
            title={title(row)}
          >
            <Metadata>
              <time dateTime={row.occurredAt}>
                {new Intl.DateTimeFormat(navigator.language, {
                  dateStyle: "medium",
                  timeStyle: "medium",
                }).format(new Date(row.occurredAt))}
              </time>
            </Metadata>
            <Observation row={row} />
          </Disclosure>
        ))}
        {!matching.length && (
          <EmptyState
            icon={Activity}
            size="compact"
            title={
              rows.length
                ? "No records on this page match this filter."
                : "No recorded trajectory observations."
            }
            sub={
              rows.length
                ? "Clear the filter or select All observations."
                : "Refresh after a Run records observations."
            }
          />
        )}
      </RecordGroup>
      <Metadata>
        Counts and filters apply to this page. Missing provider evidence stays
        unrecorded; timestamps do not imply causality. Export complete Session
        trajectory from the trusted command menu.
      </Metadata>
    </>
  );
}
type AcceptedPage = {
  page: Page<TrajectoryEntry>;
  cursors: (string | undefined)[];
  includeDescendants: boolean;
};
type Read = {
  loading: boolean;
  accepted?: AcceptedPage;
  error?: string;
};

export function Trajectory({ context }: { context: ComponentContext }) {
  const sessionId = useMemo(() => {
    if (!context.sessionId)
      throw new Error("Session trajectory requires a Session context");
    return context.sessionId;
  }, [context]);
  const [read, setRead] = useState<Read>({ loading: true });
  const reading = useRef<AbortController>(undefined);
  const load = useCallback(
    async (
      cursors: (string | undefined)[] = [undefined],
      capturedDescendants?: boolean,
    ) => {
      reading.current?.abort();
      const controller = new AbortController();
      reading.current = controller;
      const signal = AbortSignal.any([context.signal, controller.signal]);
      setRead((previous) => ({ ...previous, loading: true, error: undefined }));
      try {
        const includeDescendants =
          capturedDescendants ??
          Boolean(
            (await context.track(context.runtime.runtime.discover(signal)))
              .capabilities.features?.subagents?.enabled,
          );
        signal.throwIfAborted();
        const page = await context.track(
          context.runtime.sessions.trajectory(
            {
              sessionId,
              ...(includeDescendants ? { includeDescendants: true } : {}),
              cursor: cursors.at(-1),
              limit: 100,
            },
            signal,
          ),
        );
        if (!signal.aborted)
          setRead({
            loading: false,
            accepted: { page, cursors, includeDescendants },
          });
      } catch (error) {
        if (!signal.aborted)
          setRead((previous) => ({
            ...previous,
            loading: false,
            error:
              error instanceof Error
                ? error.message
                : "Recorded observations could not be read.",
          }));
      }
    },
    [context, sessionId],
  );
  useEffect(() => {
    void load();
    return () => reading.current?.abort();
  }, [load]);
  const accepted = read.accepted;
  return (
    <Panel label="Session trajectory">
      <Toolbar label="Trajectory read actions">
        <PanelTitle>Session trajectory</PanelTitle>
        <Button disabled={read.loading} onClick={() => void load()}>
          Refresh
        </Button>
      </Toolbar>
      <Metadata>
        Recorded Run, model and Item evidence for Session {sessionId}.
      </Metadata>
      {read.loading && <p role="status">Reading recorded observations…</p>}
      {read.error && (
        <p role="alert">The requested page could not be read: {read.error}</p>
      )}
      {accepted && (
        <>
          <p role="status">
            {accepted.page.data.length} recorded observations loaded on page{" "}
            {accepted.cursors.length}.
            {read.error && " Showing the last accepted page."}
          </p>
          <TrajectoryReport page={accepted.page} />
          <Toolbar label="Trajectory pages">
            <Button
              disabled={read.loading || accepted.cursors.length === 1}
              onClick={() =>
                void load(
                  accepted.cursors.slice(0, -1),
                  accepted.includeDescendants,
                )
              }
            >
              Newer records
            </Button>
            <Button
              disabled={read.loading || !accepted.page.nextCursor}
              onClick={() =>
                void load(
                  [...accepted.cursors, accepted.page.nextCursor!],
                  accepted.includeDescendants,
                )
              }
            >
              Older records
            </Button>
          </Toolbar>
        </>
      )}
    </Panel>
  );
}

export function RenameSession({ context }: { context: ComponentContext }) {
  const session = context.sessionTarget;
  if (!session) throw new Error("A captured Session action target is required");
  const [title, setTitle] = useState(session.title);
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<string>();
  const [unknown, setUnknown] = useState<MutationPromise<Session>>();
  const [saved, setSaved] = useState<Session>();
  const pending = useRef(false);
  const observe = async (command: MutationPromise<Session>) => {
    pending.current = true;
    setBusy(true);
    setFailure(undefined);
    try {
      const result = await context.track(command);
      if (context.signal.aborted) return;
      setSaved(result);
      setUnknown(undefined);
    } catch (error) {
      if (context.signal.aborted) return;
      setFailure(error instanceof Error ? error.message : String(error));
      setUnknown(mutationSettlementIsUnknown(error) ? command : undefined);
    } finally {
      pending.current = false;
      if (!context.signal.aborted) setBusy(false);
    }
  };
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        if (pending.current || unknown || saved || context.signal.aborted)
          return;
        context.signal.throwIfAborted();
        void observe(
          context.runtime.sessions.update({
            sessionId: session.id,
            expectedRevision: session.revision,
            title,
          }),
        );
      }}
    >
      <Metadata>
        Session {session.id} · Captured revision {session.revision}
      </Metadata>
      <TextField
        aria-label="Session title"
        value={title}
        disabled={busy || Boolean(unknown) || Boolean(saved)}
        onChange={(event) => setTitle(event.target.value)}
      />
      {failure && (
        <p role="alert">
          {failure} The captured revision remains unchanged; reopen this action
          to inspect the current Session.
        </p>
      )}
      {unknown && (
        <>
          <p role="status">
            The change is unconfirmed. Reconcile the original command before
            submitting another edit.
          </p>
          <Button
            type="button"
            disabled={busy}
            onClick={() => {
              if (pending.current || context.signal.aborted) return;
              void observe(unknown.retry());
            }}
          >
            Reconcile original command
          </Button>
        </>
      )}
      {saved && (
        <p role="status">
          Saved {saved.id} · Revision {saved.revision} ·{" "}
          {saved.title || "Untitled"}. Close and reopen to make another edit.
        </p>
      )}
      <Button
        type="submit"
        disabled={
          busy || Boolean(unknown) || Boolean(saved) || context.signal.aborted
        }
      >
        Save title
      </Button>
    </form>
  );
}

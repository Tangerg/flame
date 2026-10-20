import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  mutationSettlementIsUnknown,
  type ComponentContext,
} from "@flame/component-sdk";
import type { MutationPromise } from "@flame/runtime-client";
import type { ReactNode } from "react";
import {
  Activity,
  Refresh,
  Cpu,
  Terminal,
  FileText,
  Server,
} from "@flame/icons/react";
import type {
  Page,
  Session,
  TrajectoryEntry,
} from "@flame/runtime-client/wire";
import {
  Button,
  Badge,
  Panel,
  PanelHeader,
  PanelSection,
  PanelFooter,
  IconButton,
  Form,
  FormField,
  FormSection,
  FormActions,
  CodeBlock,
  Disclosure,
  EmptyState,
  EvidenceImage,
  Facts,
  Metadata,
  RecordBody,
  RecordGroup,
  RecordMeta,
  RecordSummary,
  SearchField,
  FilterBar,
  Select,
  Toolbar,
  TextField,
  type Fact,
  type Tone,
} from "@flame/ui";

const runId = (row: TrajectoryEntry) =>
  row.type === "run"
    ? row.run.id
    : row.type === "model"
      ? row.model.runId
      : row.type === "endpoint"
        ? (row.endpoint.runId ?? "")
        : row.item.runId;
function needsAttention(row: TrajectoryEntry): boolean {
  if (row.type === "model" || row.type === "endpoint") {
    const source = row.type === "model" ? row.model : row.endpoint;
    return source.state === "failed" || source.state === "unknown";
  }
  if (row.type === "run")
    return (
      row.run.status === "waiting" ||
      ["failed", "timedOut", "lost"].includes(row.run.outcome?.type ?? "")
    );
  return (
    row.item.status === "incomplete" ||
    (row.item.type === "toolCall" &&
      (row.item.approvalDecision === "deny" || Boolean(row.item.error)))
  );
}
function statusTone(row: TrajectoryEntry): Tone {
  if (row.type === "run") {
    switch (row.run.outcome?.type ?? row.run.status) {
      case "running":
        return "accent";
      case "waiting":
        return "warning";
      case "failed":
      case "timedOut":
      case "lost":
        return "negative";
      case "completed":
      case "canceled":
      case "finished":
        return "neutral";
    }
  }
  if (row.type === "model" || row.type === "endpoint") {
    const source = row.type === "model" ? row.model : row.endpoint;
    switch (source.state) {
      case "started":
        return "accent";
      case "completed":
        return "neutral";
      case "failed":
        return "negative";
      case "unknown":
        return "warning";
    }
  }
  const item = row.item;
  if (item.type === "toolCall") {
    if (
      item.approvalDecision === "deny" ||
      item.error?.type === "denied_by_user" ||
      item.error?.type === "tool_canceled" ||
      item.error?.type === "child_run_canceled"
    )
      return "neutral";
    if (item.error) return "negative";
  }
  switch (item.status) {
    case "running":
      return "accent";
    case "completed":
      return "neutral";
    case "incomplete":
      return "warning";
  }
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
function identity(row: TrajectoryEntry): string {
  return row.type === "run"
    ? row.run.id
    : row.type === "model"
      ? row.model.callId
      : row.type === "endpoint"
        ? row.endpoint.id
        : row.item.id;
}
const ITEM_TITLE = {
  agentMessage: "Agent response",
  userMessage: "User message",
  reasoning: "Reasoning",
  question: "Question",
  compaction: "Context compaction",
};
function title(row: TrajectoryEntry) {
  let title: string;
  let state: string;
  let decision: string | undefined;
  if (row.type === "run") {
    title = `Run · ${row.run.provider}/${row.run.model}`;
    state = row.run.outcome?.type ?? row.run.status;
  } else if (row.type === "model") {
    title = "Model call";
    state = row.model.state;
  } else if (row.type === "endpoint") {
    title = `Model endpoint · ${row.endpoint.provider}/${row.endpoint.model}`;
    state = row.endpoint.state;
  } else {
    const item = row.item;
    title = item.type === "toolCall" ? item.tool.name : ITEM_TITLE[item.type];
    state = item.status;
    if (item.type === "toolCall") decision = item.approvalDecision;
  }
  const tone = statusTone(row);
  return (
    <RecordSummary
      icon={
        row.type === "run"
          ? Activity
          : row.type === "model"
            ? Cpu
            : row.type === "endpoint"
              ? Server
              : row.item.type === "toolCall"
                ? Terminal
                : FileText
      }
      title={title}
      description={
        <RecordMeta>
          {tone === "neutral" && <span>{state}</span>}
          {row.type === "run" && <code>{row.run.id}</code>}
          <time dateTime={row.occurredAt}>
            {new Intl.DateTimeFormat(navigator.language, {
              hour: "numeric",
              minute: "2-digit",
              second: "2-digit",
            }).format(new Date(row.occurredAt))}
          </time>
          {decision && <span>Approval: {decision}</span>}
        </RecordMeta>
      }
      status={tone !== "neutral" && <Badge tone={tone}>{state}</Badge>}
    />
  );
}
function Observation({ row: recorded }: { row: TrajectoryEntry }) {
  const row = evidence(recorded);
  const body: ReactNode[] = [];
  if (row.type === "item") {
    const item = row.item;
    if (item.type === "userMessage" || item.type === "agentMessage") {
      for (const [index, part] of (item.content ?? []).entries()) {
        if (part.type === "text") {
          if (part.text)
            body.push(<RecordBody key={index}>{part.text}</RecordBody>);
        } else {
          body.push(
            <EvidenceImage
              key={index}
              src={`data:${part.mime};base64,${part.data}`}
              alt={`Recorded ${part.mime} image`}
            />,
          );
        }
      }
    } else {
      const text =
        item.type === "reasoning"
          ? item.text
          : item.type === "compaction"
            ? item.summary
            : undefined;
      if (text) body.push(<RecordBody key="text">{text}</RecordBody>);
    }
  }
  const facts: Fact[] =
    row.type === "run"
      ? [["Run ID", row.run.id]]
      : [
          ["Record ID", identity(row)],
          ["Run ID", runId(row) || "Session only"],
        ];
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
  } else if (row.type === "endpoint") {
    facts.push(
      ["Calling operation", row.endpoint.caller.id],
      ["Parent operation", row.endpoint.caller.parent ?? "Unrecorded"],
      ["Origin", row.endpoint.caller.origin],
      ["Cost (USD)", row.endpoint.costUSD ?? "Unpriced"],
      ["Recorded settlement", row.endpoint.settledAt ?? "Unrecorded"],
    );
    if (row.endpoint.failure) notes.push(row.endpoint.failure);
    if (row.endpoint.state === "unknown")
      notes.push(
        "The provider outcome was not observed. Missing usage does not mean zero consumption.",
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
  const raw = (
    <CodeBlock label="Recorded evidence" language="json">
      {JSON.stringify(row, null, 2)}
    </CodeBlock>
  );
  return (
    <>
      {body}
      <Facts values={facts} />
      {notes.map((note, index) => (
        <Metadata key={index}>{note}</Metadata>
      ))}
      {body.length ? (
        <Disclosure title="Recorded evidence">{raw}</Disclosure>
      ) : (
        raw
      )}
    </>
  );
}
export function TrajectoryReport({
  page,
  kind,
  run,
  search,
}: {
  page: Page<TrajectoryEntry>;
  kind: string;
  run: string;
  search: string;
}) {
  const rows = page.data;
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
      (run === "all" || runId(row) === run) &&
      JSON.stringify(evidence(row), (key, value) =>
        key === "data" ? undefined : value,
      )
        .toLocaleLowerCase()
        .includes(search.toLocaleLowerCase())
    );
  });
  const toolCalls = rows.filter(
    (row) => row.type === "item" && row.item.type === "toolCall",
  ).length;
  const attention = rows.filter(needsAttention).length;
  return (
    <>
      <PanelSection
        title="Recorded activity"
        description={
          <RecordMeta>
            <span>
              {matching.length === rows.length
                ? `${rows.length} ${rows.length === 1 ? "record" : "records"} on this page`
                : `${matching.length} of ${rows.length} ${rows.length === 1 ? "record" : "records"} on this page`}
            </span>
            <span>
              {toolCalls} {toolCalls === 1 ? "tool call" : "tool calls"}
            </span>
            <span>
              {attention} {attention === 1 ? "needs" : "need"} attention
            </span>
          </RecordMeta>
        }
      >
        <RecordGroup
          variant="list"
          id="records"
          aria-label="Trajectory observations"
        >
          {matching.map((row) => (
            <Disclosure
              variant="row"
              key={`${row.type}:${identity(row)}`}
              data-trajectory-record={`${row.type}:${identity(row)}`}
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
        <Disclosure title="Model activity">
          <Facts
            values={[
              [
                "Logical model calls",
                rows.filter((row) => row.type === "model").length,
              ],
              [
                "Model endpoints",
                rows.filter((row) => row.type === "endpoint").length,
              ],
            ]}
          />
        </Disclosure>
      </PanelSection>
      <Disclosure title="About these records">
        <Metadata>
          Counts and filters apply to this page. Missing provider evidence stays
          unrecorded; timestamps do not imply causality. Export complete Session
          trajectory from the trusted command menu.
        </Metadata>
      </Disclosure>
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
  const [kind, setKind] = useState("all");
  const [run, setRun] = useState("all");
  const [search, setSearch] = useState("");
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
  const rows = accepted?.page.data ?? [];
  const runOptions = [
    { value: "all", label: "All Runs" },
    ...Array.from(new Set(rows.map(runId))).map((id) => ({
      value: id,
      label: id || "Session only",
    })),
  ];
  const selectedRun = runOptions.some((option) => option.value === run)
    ? run
    : "all";

  return (
    <Panel
      label="Session trajectory"
      header={
        <PanelHeader
          size="md"
          title="Session trajectory"
          context={
            <Metadata>
              Session <code>{sessionId}</code>
            </Metadata>
          }
        />
      }
      controls={
        <FilterBar
          label="Trajectory controls"
          search={
            <SearchField
              size="lg"
              id="search"
              aria-label="Filter this page"
              placeholder="Recorded text or ID"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              onClear={() => setSearch("")}
              clearLabel="Clear filter"
            />
          }
          actions={
            <IconButton
              icon={Refresh}
              aria-label="Refresh"
              size="md"
              disabled={read.loading}
              onClick={() => void load()}
            />
          }
        >
          <Select
            size="lg"
            id="kind"
            label="Observation type"
            value={kind}
            onValueChange={setKind}
            options={[
              { value: "all", label: "All observations" },
              { value: "run", label: "Runs" },
              { value: "model", label: "Logical model calls" },
              { value: "endpoint", label: "Model endpoints" },
              { value: "toolCall", label: "Tool calls" },
              { value: "message", label: "Messages" },
              { value: "attention", label: "Needs attention" },
            ]}
          />
          <Select
            size="lg"
            id="run"
            label="Run filter"
            value={selectedRun}
            onValueChange={setRun}
            options={runOptions}
          />
        </FilterBar>
      }
    >
      {read.loading && <p role="status">Reading recorded observations…</p>}
      {read.error && (
        <p role="alert">The requested page could not be read: {read.error}</p>
      )}
      {accepted && (
        <>
          <Metadata
            role="status"
            className={read.error ? undefined : "sr-only"}
          >
            {accepted.page.data.length} recorded observations loaded on page{" "}
            {accepted.cursors.length}.
            {read.error && " Showing the last accepted page."}
          </Metadata>
          <TrajectoryReport
            page={accepted.page}
            kind={kind}
            run={selectedRun}
            search={search}
          />
          {(accepted.cursors.length > 1 ||
            Boolean(accepted.page.nextCursor)) && (
            <PanelFooter>
              {" "}
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
            </PanelFooter>
          )}
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
    <Form
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
      <FormSection title="Session details">
        <FormField label="Session title">
          <TextField
            aria-label="Session title"
            // oxlint-disable-next-line jsx-a11y/no-autofocus
            autoFocus
            value={title}
            disabled={busy || Boolean(unknown) || Boolean(saved)}
            onChange={(event) => setTitle(event.target.value)}
          />
        </FormField>
        {failure && (
          <p role="alert">
            {failure} The captured revision remains unchanged; reopen this
            action to inspect the current Session.
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
        <Disclosure title="Captured session">
          <Facts
            values={[
              ["Session", session.id],
              ["Revision", session.revision],
            ]}
          />
        </Disclosure>
      </FormSection>
      <FormActions label="Save session title">
        <Button
          variant="primary"
          type="submit"
          disabled={
            busy || Boolean(unknown) || Boolean(saved) || context.signal.aborted
          }
        >
          Save title
        </Button>
      </FormActions>
    </Form>
  );
}

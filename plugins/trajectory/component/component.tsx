import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  failureText,
  createTranslator,
  mutationSettlementIsUnknown,
  type ComponentContext,
  type Translate,
} from "@flame/component-sdk";
import type { MutationPromise } from "@flame/runtime-client";
import type { ReactNode, Ref } from "react";
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
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { de } from "./locales/de";

const CATALOGS = { en, zh, "zh-TW": zhTW, ja, ko, es, fr, de };

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
function searchable(row: TrajectoryEntry): string {
  const recorded = evidence(row);
  if (recorded.type !== "item") return JSON.stringify(recorded);
  const item = recorded.item;
  if (
    (item.type !== "userMessage" && item.type !== "agentMessage") ||
    !item.content
  )
    return JSON.stringify(recorded);
  // Only canonical message image bytes are opaque encoding; every other data
  // property, including tool arguments and results, is recorded evidence.
  const content = item.content.map((part) => {
    if (part.type !== "image") return part;
    const { data: _bytes, ...metadata } = part;
    return metadata;
  });
  return JSON.stringify({ ...recorded, item: { ...item, content } });
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
function title(row: TrajectoryEntry, t: Translate, clock: Intl.DateTimeFormat) {
  let title: string;
  let state: string;
  let decision: string | undefined;
  if (row.type === "run") {
    title = t("title.run", {
      provider: row.run.provider,
      model: row.run.model,
    });
    state = row.run.outcome?.type ?? row.run.status;
  } else if (row.type === "model") {
    title = t("title.model");
    state = row.model.state;
  } else if (row.type === "endpoint") {
    title = t("title.endpoint", {
      provider: row.endpoint.provider,
      model: row.endpoint.model,
    });
    state = row.endpoint.state;
  } else {
    const item = row.item;
    title = item.type === "toolCall" ? item.tool.name : t(`item.${item.type}`);
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
            {clock.format(new Date(row.occurredAt))}
          </time>
          {decision && <span>{t("approval.inline", { decision })}</span>}
        </RecordMeta>
      }
      status={tone !== "neutral" && <Badge tone={tone}>{state}</Badge>}
    />
  );
}
function Observation({
  row: recorded,
  t,
}: {
  row: TrajectoryEntry;
  t: Translate;
}) {
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
              alt={t("image.alt", { mime: part.mime })}
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
      ? [[t("fact.runId"), row.run.id]]
      : [
          [t("fact.recordId"), identity(row)],
          [t("fact.runId"), runId(row) || t("value.sessionOnly")],
        ];
  const notes: ReactNode[] = [];
  if (row.type === "model") {
    const source = row.model;
    facts.push(
      [t("fact.segmentId"), source.segmentId],
      [t("fact.settlement"), source.settledAt ?? t("value.unrecorded")],
    );
    if (["completed", "failed"].includes(source.state) && source.settledAt) {
      const duration =
        Date.parse(source.settledAt) - Date.parse(source.startedAt);
      if (Number.isFinite(duration) && duration >= 0)
        facts.push([t("fact.wallDurationMs"), duration]);
    }
    facts.push(
      [
        t("fact.firstOutputMs"),
        source.firstOutputLatencyMillis ?? t("value.unrecorded"),
      ],
      [
        t("fact.inputTokens"),
        source.usage?.inputTokens ?? t("value.unrecorded"),
      ],
      [
        t("fact.outputTokens"),
        source.usage?.outputTokens ?? t("value.unrecorded"),
      ],
    );
    for (const [key, label] of [
      ["cacheReadTokens", "fact.cacheReadTokens"],
      ["cacheWriteTokens", "fact.cacheWriteTokens"],
      ["reasoningTokens", "fact.reasoningTokens"],
    ] as const)
      if (source.usage?.[key] !== undefined)
        facts.push([t(label), source.usage[key]]);
    if (!source.usage) notes.push(t("note.usageMissing"));
    if (source.state === "unknown") notes.push(t("note.outcomeUnobserved"));
  } else if (row.type === "endpoint") {
    facts.push(
      [t("fact.callingOperation"), row.endpoint.caller.id],
      [
        t("fact.parentOperation"),
        row.endpoint.caller.parent ?? t("value.unrecorded"),
      ],
      [t("fact.origin"), row.endpoint.caller.origin],
      [t("fact.costUsd"), row.endpoint.costUSD ?? t("value.unpriced")],
      [t("fact.settlement"), row.endpoint.settledAt ?? t("value.unrecorded")],
    );
    if (row.endpoint.failure) notes.push(row.endpoint.failure);
    if (row.endpoint.state === "unknown")
      notes.push(t("note.providerUnobserved"));
  } else if (row.type === "run") {
    facts.push([
      t("fact.rootRunId"),
      row.run.rootRunId ?? t("value.unrecorded"),
    ]);
    if (row.run.parentRunId)
      facts.push([t("fact.parentRunId"), row.run.parentRunId]);
    if (row.run.spawnedByItemId)
      facts.push([t("fact.delegatingItemId"), row.run.spawnedByItemId]);
  } else if (row.item.type === "toolCall") {
    facts.push([
      t("fact.executionDurationMs"),
      row.item.durationMillis ?? t("value.unrecorded"),
    ]);
    if (row.item.approvalDecision)
      facts.push([t("fact.approval"), row.item.approvalDecision]);
  }
  if (row.type === "item" && row.item.type === "reasoning" && row.item.redacted)
    notes.push(t("note.reasoningRedacted"));
  const raw = (
    <CodeBlock label={t("evidence.recorded")} language="json">
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
        <Disclosure title={t("evidence.recorded")}>{raw}</Disclosure>
      ) : (
        raw
      )}
    </>
  );
}
export function TrajectoryReport({
  page,
  pageNumber,
  kind,
  run,
  search,
  stale,
  onClearFilters,
  ref,
  t,
  locale,
}: {
  page: Page<TrajectoryEntry>;
  pageNumber: number;
  kind: string;
  run: string;
  search: string;
  stale?: boolean;
  onClearFilters: () => void;
  ref?: Ref<HTMLParagraphElement>;
  t: Translate;
  locale: string;
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
      searchable(row).toLocaleLowerCase().includes(search.toLocaleLowerCase())
    );
  });
  const toolCalls = rows.filter(
    (row) => row.type === "item" && row.item.type === "toolCall",
  ).length;
  const attention = rows.filter(needsAttention).length;
  const clock = useMemo(
    () =>
      new Intl.DateTimeFormat(locale, {
        hour: "numeric",
        minute: "2-digit",
        second: "2-digit",
      }),
    [locale],
  );
  const occurred = useMemo(
    () =>
      new Intl.DateTimeFormat(locale, {
        dateStyle: "medium",
        timeStyle: "medium",
      }),
    [locale],
  );
  return (
    <>
      <Metadata ref={ref} tabIndex={-1} role="status">
        <RecordMeta>
          <span>
            {matching.length === rows.length
              ? t("records.onPage", { page: pageNumber, count: rows.length })
              : t("records.ofTotal", {
                  page: pageNumber,
                  count: rows.length,
                  matching: matching.length,
                })}
          </span>
          <span>{t("toolCalls", { count: toolCalls })}</span>
          <span>{t("attention", { count: attention })}</span>
          {stale && <span>{t("summary.stale")}</span>}
        </RecordMeta>
      </Metadata>
      <RecordGroup variant="list" id="records" aria-label={t("records.label")}>
        {matching.map((row) => (
          <Disclosure
            variant="row"
            key={`${row.type}:${identity(row)}`}
            data-trajectory-record={`${row.type}:${identity(row)}`}
            data-trajectory-kind={
              row.type === "item" ? row.item.type : row.type
            }
            title={title(row, t, clock)}
          >
            <Metadata>
              <time dateTime={row.occurredAt}>
                {occurred.format(new Date(row.occurredAt))}
              </time>
            </Metadata>
            <Observation row={row} t={t} />
          </Disclosure>
        ))}
        {!matching.length && (
          <EmptyState
            icon={Activity}
            size="compact"
            title={
              rows.length ? t("empty.filtered.title") : t("empty.none.title")
            }
            action={
              (kind !== "all" || run !== "all" || search !== "") && (
                <Button variant="outline" onClick={onClearFilters}>
                  {t("filters.clear")}
                </Button>
              )
            }
          />
        )}
      </RecordGroup>
      <Disclosure title={t("modelActivity.title")}>
        <Facts
          values={[
            [
              t("fact.logicalModelCalls"),
              rows.filter((row) => row.type === "model").length,
            ],
            [
              t("fact.modelEndpoints"),
              rows.filter((row) => row.type === "endpoint").length,
            ],
          ]}
        />
      </Disclosure>
      <Disclosure title={t("about.title")}>
        <Metadata>{t("about.note")}</Metadata>
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
  const t = useMemo(
    () => createTranslator(CATALOGS, context.locale),
    [context.locale],
  );
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
  const arrival = useRef<HTMLElement>(undefined);
  const summary = useRef<HTMLParagraphElement>(null);
  const searchElement = useRef<HTMLElement>(null);
  const load = useCallback(
    async (
      cursors: (string | undefined)[] = [undefined],
      capturedDescendants?: boolean,
      origin?: HTMLElement,
    ) => {
      reading.current?.abort();
      arrival.current = undefined;
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
        if (!signal.aborted) {
          arrival.current = origin;
          setRead({
            loading: false,
            accepted: { page, cursors, includeDescendants },
          });
        }
      } catch (error) {
        if (!signal.aborted)
          setRead((previous) => ({
            ...previous,
            loading: false,
            error: failureText(error, t("read.failed")),
          }));
      }
    },
    [context, sessionId, t],
  );
  useEffect(() => {
    void load();
    return () => reading.current?.abort();
  }, [load]);
  const accepted = read.accepted;
  useLayoutEffect(() => {
    const origin = arrival.current;
    arrival.current = undefined;
    const active = origin?.ownerDocument.activeElement;
    if (origin && (active === origin || active === origin.ownerDocument.body))
      summary.current?.focus();
  }, [accepted]);
  const rows = accepted?.page.data ?? [];
  const runOptions = [
    { value: "all", label: t("run.all") },
    ...Array.from(
      new Set([...rows.map(runId), ...(run === "all" ? [] : [run])]),
    ).map((id) => ({
      value: id,
      label: id || t("value.sessionOnly"),
    })),
  ];

  return (
    <Panel
      label={t("panel.title")}
      header={
        <PanelHeader
          size="md"
          title={t("panel.title")}
          context={
            <Metadata>
              {t("panel.session")} <code>{sessionId}</code>
            </Metadata>
          }
        />
      }
      controls={
        <FilterBar
          label={t("controls.label")}
          search={
            <SearchField
              ref={searchElement}
              size="lg"
              id="search"
              aria-label={t("search.label")}
              placeholder={t("search.placeholder")}
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              onClear={() => setSearch("")}
              clearLabel={t("search.clear")}
            />
          }
          actions={
            <IconButton
              icon={Refresh}
              aria-label={t("refresh")}
              size="md"
              pending={read.loading}
              onClick={() =>
                void (accepted
                  ? load(accepted.cursors, accepted.includeDescendants)
                  : load())
              }
            />
          }
        >
          <Select
            size="lg"
            id="kind"
            label={t("kind.label")}
            value={kind}
            onValueChange={setKind}
            options={[
              { value: "all", label: t("kind.all") },
              { value: "run", label: t("kind.run") },
              { value: "model", label: t("kind.model") },
              { value: "endpoint", label: t("kind.endpoint") },
              { value: "toolCall", label: t("kind.toolCall") },
              { value: "message", label: t("kind.message") },
              { value: "attention", label: t("kind.attention") },
            ]}
          />
          <Select
            size="lg"
            id="run"
            label={t("run.label")}
            value={run}
            onValueChange={setRun}
            options={runOptions}
          />
        </FilterBar>
      }
    >
      {read.loading && <p role="status">{t("status.reading")}</p>}
      {read.error && (
        <p role="alert">{t("status.error", { error: read.error })}</p>
      )}
      {accepted && (
        <>
          <TrajectoryReport
            ref={summary}
            page={accepted.page}
            pageNumber={accepted.cursors.length}
            kind={kind}
            run={run}
            search={search}
            stale={Boolean(read.error)}
            onClearFilters={() => {
              setKind("all");
              setRun("all");
              setSearch("");
              searchElement.current?.focus();
            }}
            t={t}
            locale={context.locale}
          />
          {(accepted.cursors.length > 1 ||
            Boolean(accepted.page.nextCursor)) && (
            <PanelFooter>
              {" "}
              <Toolbar label={t("pages.label")}>
                <Button
                  pending={read.loading}
                  disabled={accepted.cursors.length === 1}
                  onClick={(event) =>
                    void load(
                      accepted.cursors.slice(0, -1),
                      accepted.includeDescendants,
                      event.currentTarget,
                    )
                  }
                >
                  {t("pages.newer")}
                </Button>
                <Button
                  pending={read.loading}
                  disabled={!accepted.page.nextCursor}
                  onClick={(event) =>
                    void load(
                      [...accepted.cursors, accepted.page.nextCursor!],
                      accepted.includeDescendants,
                      event.currentTarget,
                    )
                  }
                >
                  {t("pages.older")}
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
  const t = useMemo(
    () => createTranslator(CATALOGS, context.locale),
    [context.locale],
  );
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
      setFailure(failureText(error, String(error)));
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
      <FormSection title={t("rename.section")}>
        <FormField label={t("rename.titleLabel")}>
          <TextField
            aria-label={t("rename.titleLabel")}
            // oxlint-disable-next-line jsx-a11y/no-autofocus
            autoFocus
            value={title}
            pending={busy}
            readOnly={Boolean(unknown) || Boolean(saved)}
            onChange={(event) => setTitle(event.target.value)}
          />
        </FormField>
        {failure && (
          <p role="alert">{t("rename.failure", { message: failure })}</p>
        )}
        {unknown && (
          <>
            <p role="status">{t("rename.unconfirmed")}</p>
            <Button
              type="button"
              pending={busy}
              aria-busy={busy || undefined}
              onClick={() => {
                if (pending.current || context.signal.aborted) return;
                void observe(unknown.retry());
              }}
            >
              {busy ? (
                <span role="status">{t("rename.checking")}</span>
              ) : (
                t("rename.reconcile")
              )}
            </Button>
          </>
        )}
        {saved && (
          <p role="status">
            {t("rename.saved", {
              id: saved.id,
              revision: saved.revision,
              title: saved.title || t("rename.untitled"),
            })}
          </p>
        )}
        <Disclosure title={t("rename.captured")}>
          <Facts
            values={[
              [t("panel.session"), session.id],
              [t("rename.factRevision"), session.revision],
            ]}
          />
        </Disclosure>
      </FormSection>
      <FormActions label={t("rename.actions")}>
        <Button
          variant="primary"
          type="submit"
          pending={busy}
          focusableWhenDisabled
          aria-busy={(busy && !unknown) || undefined}
          disabled={
            Boolean(unknown) || Boolean(saved) || context.signal.aborted
          }
        >
          {busy && !unknown ? (
            <span role="status">{t("rename.saving")}</span>
          ) : (
            t("rename.save")
          )}
        </Button>
      </FormActions>
    </Form>
  );
}

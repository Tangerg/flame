import { useMemo } from "react";
import type { ReactNode, Ref } from "react";
import type { Translate } from "@flame/component-sdk";
import { Activity, Cpu, Terminal, FileText, Server } from "@flame/icons/react";
import type { Page, TrajectoryEntry } from "@flame/runtime-client/wire";
import { prettyJSON, stringifyJSON } from "@qimu/sdk/json";
import {
  Button,
  Badge,
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
  type Fact,
  type Tone,
} from "@flame/ui";

export const runId = (row: TrajectoryEntry) =>
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
  if (recorded.type !== "item") return stringifyJSON(recorded);
  const item = recorded.item;
  if (
    (item.type !== "userMessage" && item.type !== "agentMessage") ||
    !item.content
  )
    return stringifyJSON(recorded);
  // Only canonical message image bytes are opaque encoding; every other data
  // property, including tool arguments and results, is recorded evidence.
  const content = item.content.map((part) => {
    if (part.type !== "image") return part;
    const { data: _bytes, ...metadata } = part;
    return metadata;
  });
  return stringifyJSON({ ...recorded, item: { ...item, content } });
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
              labels={{
                view: t("image.view"),
                close: t("image.close"),
                zoomIn: t("image.zoomIn"),
                zoomOut: t("image.zoomOut"),
                description: t("image.description"),
              }}
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
      {prettyJSON(row)}
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

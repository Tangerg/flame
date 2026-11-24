import type { Translate } from "@flame/component-sdk";
import type { TrajectoryEntry } from "@flame/runtime-client/wire";
import { stringifyJSON } from "@frame/sdk/json";
import { type Tone } from "@flame/ui";

export const runId = (row: TrajectoryEntry) =>
  row.type === "run"
    ? row.run.id
    : row.type === "model"
      ? row.model.runId
      : row.type === "endpoint"
        ? (row.endpoint.runId ?? "")
        : row.item.runId;

// A denied or canceled tool call reads as a normal end but still deserves a look.
export function needsAttention(row: TrajectoryEntry): boolean {
  if (row.type !== "item") {
    const tone = statusTone(row);
    return tone === "warning" || tone === "negative";
  }
  return (
    row.item.status === "incomplete" ||
    (row.item.type === "toolCall" &&
      (row.item.approvalDecision === "deny" || Boolean(row.item.error)))
  );
}

export function statusTone(row: TrajectoryEntry): Tone {
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

export function evidence(row: TrajectoryEntry): TrajectoryEntry {
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

export function readable(
  row: TrajectoryEntry,
  t: Translate,
): { state: string; approval?: string; approvalInline?: string } {
  const state = t(
    `state.${
      row.type === "run"
        ? (row.run.outcome?.type ?? row.run.status)
        : row.type === "model"
          ? row.model.state
          : row.type === "endpoint"
            ? row.endpoint.state
            : row.item.status
    }`,
  );
  const decision =
    row.type === "item" && row.item.type === "toolCall"
      ? row.item.approvalDecision
      : undefined;
  if (!decision) return { state };
  const approval = t(`approval.${decision}`);
  return {
    state,
    approval,
    approvalInline: t("approval.inline", { decision: approval }),
  };
}

export function matches(
  row: TrajectoryEntry,
  query: string,
  t: Translate,
): boolean {
  const { state, approval, approvalInline } = readable(row, t);
  return [searchable(row), state, approval, approvalInline].some((text) =>
    text?.toLocaleLowerCase().includes(query),
  );
}

export function identity(row: TrajectoryEntry): string {
  return row.type === "run"
    ? row.run.id
    : row.type === "model"
      ? row.model.callId
      : row.type === "endpoint"
        ? row.endpoint.id
        : row.item.id;
}

export function recordKey(row: TrajectoryEntry): string {
  return `${row.type}:${identity(row)}`;
}

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

type RunRow = Extract<TrajectoryEntry, { type: "run" }>["run"];
type ObservationState =
  | Extract<TrajectoryEntry, { type: "model" }>["model"]["state"]
  | Extract<TrajectoryEntry, { type: "endpoint" }>["endpoint"]["state"];
type ItemRow = Extract<TrajectoryEntry, { type: "item" }>["item"];
type ItemStatus = ItemRow["status"];
type ToolCallFailure = NonNullable<
  Extract<ItemRow, { type: "toolCall" }>["error"]
>["type"];

const RUN_TONE = {
  running: "accent",
  waiting: "warning",
  failed: "negative",
  timedOut: "negative",
  lost: "negative",
  completed: "neutral",
  canceled: "neutral",
  finished: "neutral",
} as const satisfies Record<
  NonNullable<RunRow["outcome"]>["type"] | RunRow["status"],
  Tone
>;
const OBSERVATION_TONE = {
  started: "accent",
  completed: "neutral",
  failed: "negative",
  unknown: "warning",
} as const satisfies Record<ObservationState, Tone>;
const ITEM_TONE = {
  running: "accent",
  completed: "neutral",
  incomplete: "warning",
} as const satisfies Record<ItemStatus, Tone>;
// The user or a parent ended these calls on purpose; they are not failures.
const DELIBERATE_TOOL_ENDS = new Set<ToolCallFailure>([
  "denied_by_user",
  "tool_canceled",
  "child_run_canceled",
]);

export function statusTone(row: TrajectoryEntry): Tone {
  switch (row.type) {
    case "run":
      return RUN_TONE[row.run.outcome?.type ?? row.run.status];
    case "model":
      return OBSERVATION_TONE[row.model.state];
    case "endpoint":
      return OBSERVATION_TONE[row.endpoint.state];
  }
  const item = row.item;
  if (item.type === "toolCall") {
    if (
      item.approvalDecision === "deny" ||
      (item.error !== undefined && DELIBERATE_TOOL_ENDS.has(item.error.type))
    )
      return "neutral";
    if (item.error) return "negative";
  }
  return ITEM_TONE[item.status];
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

// The ordinary end of a record; rows name only the states that differ from it.
const ORDINARY_END = new Set(["completed", "finished"]);

export function readable(
  row: TrajectoryEntry,
  t: Translate,
): {
  state: string;
  ordinary: boolean;
  approval?: string;
  approvalInline?: string;
} {
  const key =
    row.type === "run"
      ? (row.run.outcome?.type ?? row.run.status)
      : row.type === "model"
        ? row.model.state
        : row.type === "endpoint"
          ? row.endpoint.state
          : row.item.status;
  const state = t(`state.${key}`);
  const ordinary = ORDINARY_END.has(key);
  const decision =
    row.type === "item" && row.item.type === "toolCall"
      ? row.item.approvalDecision
      : undefined;
  if (!decision) return { state, ordinary };
  const approval = t(`approval.${decision}`);
  return {
    state,
    ordinary,
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

import type { AgentItem, AgentRunFact } from "@/plugins/sdk";
import type { Tone } from "@/lib/tone";
import type { TrajectoryEntry } from "@/plugins/builtin/agent/public/run";

export type TimelineCategory = "all" | "run" | "model" | "toolCall" | "message" | "attention";
export type TimelineKind = "run" | "model" | AgentItem["type"];

export interface TimelineRecord {
  key: string;
  id: string;
  runId: string;
  occurredAt: string;
  kind: TimelineKind;
  summary: string;
  statusKey: string;
  tone: Tone;
  attention: boolean;
  durationMillis?: number;
  source: TrajectoryEntry;
}

export interface TimelineFilters {
  category: TimelineCategory;
  query: string;
  runId: string | null;
}

export interface TimelineViewModel {
  records: TimelineRecord[];
  recordCount: number;
  modelCount: number;
  toolCount: number;
  attentionCount: number;
  runIds: string[];
}

const MODEL_STATUS = {
  started: { statusKey: "timeline.modelCall.started", tone: "accent", attention: false },
  completed: { statusKey: "timeline.modelCall.completed", tone: "success", attention: false },
  failed: { statusKey: "timeline.modelCall.failed", tone: "negative", attention: true },
  unknown: { statusKey: "timeline.modelCall.unknown", tone: "warning", attention: true },
} as const;

const ITEM_STATUS = {
  running: { statusKey: "timeline.state.running", tone: "accent", attention: false },
  completed: { statusKey: "timeline.state.completed", tone: "neutral", attention: false },
  incomplete: { statusKey: "timeline.state.incomplete", tone: "warning", attention: true },
} as const;

export function timelineViewModel(
  entries: readonly TrajectoryEntry[],
  filters: TimelineFilters,
): TimelineViewModel {
  const records = entries.map(timelineRecord);
  const needle = filters.query.trim().toLocaleLowerCase();
  return {
    records: records.filter((record) => matches(record, filters, needle)),
    recordCount: records.length,
    modelCount: records.filter((record) => record.kind === "model").length,
    toolCount: records.filter((record) => record.kind === "toolCall").length,
    attentionCount: records.filter((record) => record.attention).length,
    runIds: [...new Set(records.map((record) => record.runId))],
  };
}

function timelineRecord(source: TrajectoryEntry): TimelineRecord {
  if (source.type === "model") {
    const call = source.model;
    return {
      key: `model:${call.callId}`,
      id: call.callId,
      runId: call.runId,
      occurredAt: source.occurredAt,
      kind: "model",
      summary: call.callId,
      ...MODEL_STATUS[call.state],
      durationMillis:
        call.state === "completed" || call.state === "failed"
          ? elapsedMillis(call.startedAt, call.settledAt)
          : undefined,
      source,
    };
  }
  if (source.type === "run") {
    const run = source.run;
    return {
      key: `run:${run.id}`,
      id: run.id,
      runId: run.id,
      occurredAt: source.occurredAt,
      kind: "run",
      summary: run.modelSelection
        ? `${run.modelSelection.provider}/${run.modelSelection.model}`
        : run.id,
      ...runStatus(run),
      durationMillis: elapsedMillis(run.createdAt, run.finishedAt ?? undefined),
      source,
    };
  }
  const item = source.item;
  return {
    key: `item:${item.id}`,
    id: item.id,
    runId: item.runId,
    occurredAt: source.occurredAt,
    kind: item.type,
    summary: itemSummary(item),
    ...itemStatus(item),
    durationMillis: item.type === "toolCall" ? item.durationMillis : undefined,
    source,
  };
}

function runStatus(run: AgentRunFact): Pick<TimelineRecord, "statusKey" | "tone" | "attention"> {
  if (run.status === "running")
    return { statusKey: "agent.runTree.status.running", tone: "accent", attention: false };
  if (run.status === "waiting")
    return { statusKey: "agent.runTree.status.waiting", tone: "warning", attention: true };
  if (run.outcome?.type === "completed")
    return { statusKey: "agent.runTree.status.finished", tone: "success", attention: false };
  if (run.outcome?.type === "canceled")
    return { statusKey: "agent.runTree.status.canceled", tone: "neutral", attention: false };
  return { statusKey: "agent.runTree.status.error", tone: "negative", attention: true };
}

function itemStatus(item: AgentItem): Pick<TimelineRecord, "statusKey" | "tone" | "attention"> {
  if (item.type !== "toolCall") return ITEM_STATUS[item.status];
  if (item.approvalDecision === "deny")
    return { statusKey: "timeline.state.deny", tone: "warning", attention: true };
  if (item.error)
    return { statusKey: "timeline.modelCall.failed", tone: "negative", attention: true };
  return ITEM_STATUS[item.status];
}

function itemSummary(item: AgentItem): string {
  switch (item.type) {
    case "toolCall":
      return item.tool.name;
    case "reasoning":
      return item.redacted ? "" : (item.text ?? "");
    case "compaction":
      return item.summary;
    case "question":
      return item.question.fields.map((field) => field.prompt).join(" · ");
    case "userMessage":
    case "agentMessage":
      return (item.content ?? [])
        .flatMap((part) => (part.type === "text" ? [part.text] : []))
        .join("\n");
  }
}

function matches(record: TimelineRecord, filters: TimelineFilters, needle: string): boolean {
  if (filters.runId !== null && record.runId !== filters.runId) return false;
  if (!matchesCategory(record, filters.category)) return false;
  if (needle === "") return true;
  return searchableText(record).toLocaleLowerCase().includes(needle);
}

function matchesCategory(record: TimelineRecord, category: TimelineCategory): boolean {
  if (category === "all") return true;
  if (category === "attention") return record.attention;
  if (category === "message")
    return (
      record.kind === "userMessage" || record.kind === "agentMessage" || record.kind === "reasoning"
    );
  return record.kind === category;
}

function searchableText(record: TimelineRecord): string {
  const source = record.source;
  const identity = `${record.id}\n${record.runId}\n${record.summary}`;
  if (source.type === "model")
    return `${identity}\n${source.model.segmentId}\n${source.model.state}`;
  if (source.type === "run")
    return `${identity}\n${source.run.parentRunId ?? ""}\n${JSON.stringify(source.run.outcome)}`;
  if (source.item.type === "toolCall")
    return `${identity}\n${JSON.stringify(source.item.tool)}\n${JSON.stringify(source.item.error)}`;
  if (source.item.type === "question")
    return `${identity}\n${JSON.stringify(source.item.question)}`;
  return identity;
}

export function elapsedMillis(start: string, end: string | undefined): number | undefined {
  if (end === undefined) return undefined;
  const elapsed = Date.parse(end) - Date.parse(start);
  return Number.isFinite(elapsed) && elapsed >= 0 ? elapsed : undefined;
}

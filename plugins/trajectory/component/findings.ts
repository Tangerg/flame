import type { ReactNode } from "react";
import type { Translate } from "@flame/component-sdk";
import type { TrajectoryEntry } from "@flame/runtime-client/wire";
import { type Fact } from "@flame/ui";
import { identity, readable, runId } from "./recordFacts";

interface Findings {
  facts: Fact[];
  notes: ReactNode[];
}

type Moment = (iso: string) => ReactNode;

function modelFindings(
  source: Extract<TrajectoryEntry, { type: "model" }>["model"],
  t: Translate,
  moment: Moment,
  { facts, notes }: Findings,
): void {
  facts.push(
    [t("fact.segmentId"), source.segmentId],
    [
      t("fact.settlement"),
      source.settledAt ? moment(source.settledAt) : t("value.unrecorded"),
    ],
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
    [t("fact.inputTokens"), source.usage?.inputTokens ?? t("value.unrecorded")],
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
  if (source.state === "unknown") notes.push(t("note.outcomeUnobserved"));
}

function endpointFindings(
  endpoint: Extract<TrajectoryEntry, { type: "endpoint" }>["endpoint"],
  t: Translate,
  moment: Moment,
  { facts, notes }: Findings,
): void {
  facts.push(
    [t("fact.callingOperation"), endpoint.caller.id],
    [
      t("fact.parentOperation"),
      endpoint.caller.parent ?? t("value.unrecorded"),
    ],
    [t("fact.origin"), endpoint.caller.origin],
    [t("fact.costUsd"), endpoint.costUsd ?? t("value.unpriced")],
    [
      t("fact.settlement"),
      endpoint.settledAt ? moment(endpoint.settledAt) : t("value.unrecorded"),
    ],
  );
  if (endpoint.failure) notes.push(endpoint.failure);
  if (endpoint.state === "unknown") notes.push(t("note.providerUnobserved"));
}

function runFacts(
  run: Extract<TrajectoryEntry, { type: "run" }>["run"],
  t: Translate,
  facts: Fact[],
): void {
  // A root run has no lineage; delegated runs record all three ids.
  if (run.rootRunId) facts.push([t("fact.rootRunId"), run.rootRunId]);
  if (run.parentRunId) facts.push([t("fact.parentRunId"), run.parentRunId]);
  if (run.spawnedByItemId)
    facts.push([t("fact.delegatingItemId"), run.spawnedByItemId]);
}

export function observationFindings(
  row: TrajectoryEntry,
  t: Translate,
  moment: Moment,
): Findings {
  const findings: Findings = {
    facts:
      row.type === "run"
        ? [[t("fact.runId"), row.run.id]]
        : [
            [t("fact.recordId"), identity(row)],
            [t("fact.runId"), runId(row) || t("value.sessionOnly")],
          ],
    notes: [],
  };
  if (row.type === "model") modelFindings(row.model, t, moment, findings);
  else if (row.type === "endpoint")
    endpointFindings(row.endpoint, t, moment, findings);
  else if (row.type === "run") runFacts(row.run, t, findings.facts);
  else if (row.item.type === "toolCall") {
    findings.facts.push([
      t("fact.executionDurationMs"),
      row.item.durationMillis ?? t("value.unrecorded"),
    ]);
    const { approval } = readable(row, t);
    if (approval) findings.facts.push([t("fact.approval"), approval]);
  }
  if (row.type === "item" && row.item.type === "reasoning" && row.item.redacted)
    findings.notes.push(t("note.reasoningRedacted"));
  return findings;
}

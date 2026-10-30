import { type RecordedFileChange } from "@flame/component-sdk";
import { CHANGE_STATUSES, type ResultShape } from "@/foundation/protocolValues.generated";
import { isJSONNumber, parseJSON, projectNumber } from "@qimu/sdk/json";

export interface CommandResult {
  exitCode?: number;
  output?: string;
}

export interface PatchResult {
  changes: RecordedFileChange[];
}

export interface SearchResult {
  hits: unknown[];
}

export interface WebSearchResult {
  results: unknown[];
}

function readToolResultObject(value: unknown): Record<string, unknown> | undefined {
  if (typeof value === "string") {
    if (!value) return undefined;
    try {
      return readToolResultObject(parseJSON(value));
    } catch {
      return undefined;
    }
  }
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

function readRepresentableNumber(value: unknown): number | undefined {
  if (!isJSONNumber(value)) return undefined;
  try {
    return projectNumber(value);
  } catch {
    return undefined;
  }
}

function readRecordedFileChange(value: unknown): RecordedFileChange | undefined {
  const change = readToolResultObject(value);
  if (!change || typeof change.path !== "string") return undefined;
  const status = CHANGE_STATUSES.find((candidate) => candidate === change.status);
  const added = readRepresentableNumber(change.added);
  const removed = readRepresentableNumber(change.removed);
  if (
    status === undefined ||
    added === undefined ||
    removed === undefined ||
    !Number.isSafeInteger(added) ||
    added < 0 ||
    !Number.isSafeInteger(removed) ||
    removed < 0 ||
    (change.from !== undefined && typeof change.from !== "string")
  )
    return undefined;
  return {
    path: change.path,
    status,
    added,
    removed,
    ...(typeof change.from === "string" ? { from: change.from } : {}),
  };
}

export function searchToolResult(raw: unknown): SearchResult | undefined {
  const value = readToolResultObject(raw);
  return Array.isArray(value?.hits) ? { hits: value.hits } : undefined;
}

export function patchToolResult(raw: unknown): PatchResult | undefined {
  const value = readToolResultObject(raw);
  if (!Array.isArray(value?.changes)) return undefined;
  const changes: RecordedFileChange[] = [];
  for (const entry of value.changes) {
    const change = readRecordedFileChange(entry);
    if (!change) return undefined;
    changes.push(change);
  }
  return { changes };
}

export function commandToolResult(raw: unknown): CommandResult | undefined {
  const value = readToolResultObject(raw);
  if (!value) return undefined;
  const output = typeof value.output === "string" ? value.output : undefined;
  const exitCode = readRepresentableNumber(value.exitCode);
  if (output === undefined && exitCode === undefined) return undefined;
  return {
    ...(output === undefined ? {} : { output }),
    ...(exitCode === undefined ? {} : { exitCode }),
  };
}

export function webSearchToolResult(raw: unknown): WebSearchResult | undefined {
  const value = readToolResultObject(raw);
  return Array.isArray(value?.results) ? { results: value.results } : undefined;
}

const RESULT_SHAPE_MATCHERS = {
  search: (value) => Array.isArray(value.hits),
  patch: (value) => Array.isArray(value.changes),
  webSearch: (value) => Array.isArray(value.results),
  command: (value) => typeof value.output === "string",
} satisfies Record<ResultShape, (value: Record<string, unknown>) => boolean>;

export function toolResultShape(raw: unknown): ResultShape | undefined {
  const value = readToolResultObject(raw);
  if (!value) return undefined;
  for (const [shape, matches] of Object.entries(RESULT_SHAPE_MATCHERS)) {
    if (matches(value)) return shape as ResultShape;
  }
  return undefined;
}

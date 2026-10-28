import { type RecordedFileChange } from "@flame/component-sdk";
import { WIRE_ENUMS } from "@flame/runtime-client/wire";
import { isJSONNumber, parseJSON, projectNumber } from "@qimu/sdk/json";

export interface CommandResult {
  exitCode?: number;
  output: string;
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
  const status = WIRE_ENUMS.ChangeStatus.find((candidate) => candidate === change.status);
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
  if (typeof value?.output !== "string") return undefined;
  const exitCode = readRepresentableNumber(value.exitCode);
  return { output: value.output, ...(exitCode === undefined ? {} : { exitCode }) };
}

export function webSearchToolResult(raw: unknown): WebSearchResult | undefined {
  const value = readToolResultObject(raw);
  return Array.isArray(value?.results) ? { results: value.results } : undefined;
}

export const TOOL_RESULT_SHAPES = ["search", "patch", "command", "webSearch"] as const;

export type ToolResultShape = (typeof TOOL_RESULT_SHAPES)[number];

export function toolResultShape(raw: unknown): ToolResultShape | undefined {
  const value = readToolResultObject(raw);
  if (Array.isArray(value?.hits)) return "search";
  if (Array.isArray(value?.changes)) return "patch";
  if (Array.isArray(value?.results)) return "webSearch";
  if (typeof value?.output === "string") return "command";
  return undefined;
}

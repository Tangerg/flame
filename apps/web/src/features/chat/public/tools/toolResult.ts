import { type RecordedFileChange } from "@flame/component-sdk";
type ChangeStatus = RecordedFileChange["status"];

interface AppliedChange {
  from?: string;
  path: string;
  status: ChangeStatus;
}

export interface CommandResult {
  exitCode?: number;
  output: string;
}

export interface PatchResult {
  changes: AppliedChange[];
}

interface SearchHit {
  lineNumber?: number;
  path: string;
  snippet?: string;
}

export interface SearchResult {
  hits: SearchHit[];
}

interface WebSearchHit {
  faviconUrl?: string;
  snippet?: string;
  title?: string;
  url: string;
}

export interface WebSearchResult {
  results: WebSearchHit[];
}

function parsed<T>(value: unknown): Partial<T> | undefined {
  if (typeof value === "string") {
    if (!value) return undefined;
    try {
      return parsed<T>(JSON.parse(value));
    } catch {
      return undefined;
    }
  }
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Partial<T>)
    : undefined;
}

export function searchToolResult(raw: unknown): Partial<SearchResult> | undefined {
  return parsed<SearchResult>(raw);
}

export function patchToolResult(raw: unknown): Partial<PatchResult> | undefined {
  return parsed<PatchResult>(raw);
}

export function commandToolResult(raw: unknown): Partial<CommandResult> | undefined {
  return parsed<CommandResult>(raw);
}

export function webSearchToolResult(raw: unknown): Partial<WebSearchResult> | undefined {
  return parsed<WebSearchResult>(raw);
}

export const TOOL_RESULT_SHAPES = ["search", "patch", "command", "webSearch"] as const;

export type ToolResultShape = (typeof TOOL_RESULT_SHAPES)[number];

export function toolResultShape(raw: unknown): ToolResultShape | undefined {
  if (Array.isArray(searchToolResult(raw)?.hits)) return "search";
  if (Array.isArray(patchToolResult(raw)?.changes)) return "patch";
  if (Array.isArray(webSearchToolResult(raw)?.results)) return "webSearch";
  const command = commandToolResult(raw);
  if (typeof command?.output === "string") return "command";
  return undefined;
}

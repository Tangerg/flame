// `ToolCall.result` is passed through as an opaque string, but its CONTENT is not opaque:
// the Runtime's `toolResultPresentations` declares a schema per tool name. Four consumers
// were each rediscovering those property names through `Record<string, unknown>` index
// access — `parsed?.hits`, `parsed?.changes`, `parsed?.exitCode` — which no type checks, so
// renaming one on the Runtime side emptied a preview and threw nothing.
//
// Reading them through the generated types makes that rename a compile error instead.
//
// `Partial<T>`, and NOT the contract's own validator, because these feed display surfaces:
// a transcript receipt with one entry the client cannot read must still show the rest, and
// whole-payload validation would blank the row instead. Callers keep their per-entry
// guards; what they no longer keep is a private guess at the property names.

import type {
  CommandResult,
  PatchResult,
  SearchResult,
  WebSearchResult,
} from "@flame/runtime-contract/wire";

// Both call paths, because the payload arrives decoded in the fold (the wire already
// parsed it) and as a string everywhere downstream (`ToolCall.result` carries it verbatim).
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

/** `glob` and `grep`: the Runtime folds every output mode into one envelope. */
export function searchToolResult(raw: unknown): Partial<SearchResult> | undefined {
  return parsed<SearchResult>(raw);
}

/** `apply_patch`: the changes THIS call applied, never the worktree. */
export function patchToolResult(raw: unknown): Partial<PatchResult> | undefined {
  return parsed<PatchResult>(raw);
}

/** `shell`: a non-zero exit is not always failure, so the code is reported, not judged. */
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

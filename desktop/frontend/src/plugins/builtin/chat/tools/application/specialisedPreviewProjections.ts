import { searchToolResult, webSearchToolResult } from "@/plugins/sdk";
import { parseJsonResult, resultLines } from "./toolResultParsing";

export interface SkillPreviewEntry {
  name: string;
  description: string;
}

export interface GlobPreviewModel {
  paths: string[];
}

export interface WebSearchPreviewResult {
  url: string;
  domain: string;
  title: string;
  snippet: string;
}

const SKILL_ENTRY = /<skill>\s*<name>([\s\S]*?)<\/name>\s*<description>([\s\S]*?)<\/description>/g;

export function projectSkillPreview(result: string | undefined): SkillPreviewEntry[] {
  return [...(result ?? "").matchAll(SKILL_ENTRY)].map((match) => ({
    name: match[1]!.trim(),
    description: match[2]!.trim(),
  }));
}

export function projectAskUserAnswer(result: string | undefined): string {
  const text = result?.trim();
  if (!text) return "";
  const parsed = parseJsonResult(result);
  if (!parsed) return text;
  const direct = parsed.answer ?? parsed.response;
  if (typeof direct === "string") return direct;
  const parts = Object.values(parsed).map((value) =>
    typeof value === "string"
      ? value
      : Array.isArray(value)
        ? value.filter((entry) => typeof entry === "string").join(", ")
        : "",
  );
  return parts.filter(Boolean).join(" · ") || text;
}

export function projectGlobPreview(result: string | undefined): GlobPreviewModel {
  const hits = searchToolResult(result)?.hits;
  if (!Array.isArray(hits)) return { paths: [] };
  return { paths: hits.map((hit) => hit?.path ?? "").filter((path) => path.length > 0) };
}

export function projectWebSearchPreview(result: string | undefined): WebSearchPreviewResult[] {
  const results = webSearchToolResult(result)?.results;
  if (!Array.isArray(results)) return [];
  return results.flatMap((hit) =>
    hit?.url
      ? [
          {
            url: hit.url,
            domain: domainOf(hit.url),
            title: hit.title || hit.url,
            snippet: hit.snippet ?? "",
          },
        ]
      : [],
  );
}

function record(value: unknown): Record<string, unknown> {
  return typeof value === "object" && value !== null ? (value as Record<string, unknown>) : {};
}

function domainOf(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, "");
  } catch {
    return url;
  }
}

// These answer in PROSE the model reads, not JSON, so each projection parses it — anchored
// on the ONE piece of structure the runtime emits and degrading to "no structure found", so
// a backend wording change costs a plain-text preview rather than a wrong one.

/** `search_memory`: `N. content`, one entry per recalled item, content may wrap. */
export function projectRecalledMemories(result: string | undefined): string[] {
  const entries: string[] = [];
  for (const line of resultLines(result)) {
    const start = /^\d+\.\s+(.*)$/.exec(line);
    if (start) entries.push(start[1]!);
    else if (entries.length > 0) entries[entries.length - 1] += `\n${line}`;
  }
  return entries;
}

export interface ConversationHit {
  speaker: string;
  day: string;
  snippet: string;
}

/** `search_conversations`: `N. [speaker · YYYY-MM-DD] snippet`. */
export function projectConversationHits(result: string | undefined): ConversationHit[] {
  const hits: ConversationHit[] = [];
  for (const line of resultLines(result)) {
    const parsed = /^\d+\.\s+\[([^·\]]+)·\s*([^\]]+)\]\s*(.*)$/.exec(line);
    if (parsed) {
      hits.push({ speaker: parsed[1]!.trim(), day: parsed[2]!.trim(), snippet: parsed[3]! });
    } else if (hits.length > 0) {
      hits[hits.length - 1]!.snippet += `\n${line}`;
    }
  }
  return hits;
}

export interface ToolSearchGroup {
  source: string;
  names: string[];
}

/** `search_tools`: prose, then `Not loaded:` and `  [source] a, b, c` per source. */
export function projectToolSearchGroups(result: string | undefined): ToolSearchGroup[] {
  const groups: ToolSearchGroup[] = [];
  for (const line of resultLines(result)) {
    const parsed = /^\s*\[([^\]]+)\]\s*(.+)$/.exec(line);
    if (!parsed) continue;
    const names = parsed[2]!
      .split(",")
      .map((name) => name.trim())
      .filter(Boolean);
    if (names.length > 0) groups.push({ source: parsed[1]!.trim(), names });
  }
  return groups;
}

export interface HttpPreview {
  status: number;
  duration: string;
  truncated: boolean;
  headers: [string, string][];
  body: string;
}

/** `http_request` answers `{status, headers, body, truncated, duration}`. */
export function projectHttpPreview(result: string | undefined): HttpPreview | undefined {
  const parsed = parseJsonResult(result);
  if (typeof parsed?.status !== "number") return undefined;
  const headers = record(parsed.headers);
  return {
    status: parsed.status,
    duration: text(parsed.duration),
    truncated: parsed.truncated === true,
    headers: Object.entries(headers).map(([name, value]) => [name, text(value)]),
    body: text(parsed.body),
  };
}

export interface FetchedPage {
  content: string;
  format: string;
}

/** `web_fetch` answers `{content, format}` — markdown by default. */
export function projectFetchedPage(result: string | undefined): FetchedPage | undefined {
  const parsed = parseJsonResult(result);
  if (typeof parsed?.content !== "string") return undefined;
  return { content: parsed.content, format: text(parsed.format) || "text" };
}

function text(value: unknown): string {
  return typeof value === "string" ? value : "";
}

import type { Translate } from "@/lib/i18n";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { fmtDuration } from "@/lib/format";
import { toolVerbId } from "@/lib/toolFamilies";
import { toolCategory } from "@/plugins/builtin/agent/domain/toolCategory";

export type ToolDetail = { kind: "path" | "machine" | "prose"; value: string };

export interface ToolIntent {
  label: ToolDetail;
  detail?: ToolDetail;
}

export type ToolMetaTone = "muted" | "negative";

export interface ToolMetaItem {
  id: string;
  label: string;
  tone: ToolMetaTone;
}

const GENERIC_VERB_ID = "generic";

const TOOL_DETAIL_KEYS: ReadonlyArray<{ key: string; kind: ToolDetail["kind"] }> = [
  { key: "path", kind: "path" },
  { key: "query", kind: "machine" },
  { key: "pattern", kind: "machine" },
  { key: "url", kind: "machine" },
];

export function toolIntent(t: Translate, tool: ToolCall): ToolIntent {
  const labelKey = toolVerbId(tool.name);
  const tense = tool.status === "running" ? "doing" : tool.status === "ok" ? "done" : "action";
  const verb: ToolDetail = {
    kind: "prose",
    value: t(`tool.${tense}.${labelKey ?? GENERIC_VERB_ID}`),
  };
  const command = text(tool.command);
  const described = command !== undefined && tool.fn !== command.value;
  const argument: ToolDetail | undefined =
    tool.fn === tool.name && labelKey !== undefined
      ? undefined
      : { kind: tool.fnKind ?? "machine", value: tool.fn };
  const label = described ? { kind: "prose" as const, value: tool.fn } : verb;
  const parsed = parseToolArgs(tool.args);
  const detail =
    command ?? (described ? undefined : argument) ?? (parsed ? toolDetail(parsed) : undefined);
  return detail && detail.value === label.value ? { label } : { label, detail };
}

export function toolMetaItems(t: Translate, tool: ToolCall): ToolMetaItem[] {
  const items: ToolMetaItem[] = [];
  if (tool.files != null) {
    items.push({ id: "files", label: t("tool.meta.files", { count: tool.files }), tone: "muted" });
  }
  if (tool.hits != null) {
    const found =
      toolCategory(tool.name) === "webSearch" ? "tool.meta.results" : "tool.meta.matches";
    items.push({ id: "hits", label: t(found, { count: tool.hits }), tone: "muted" });
  }
  if (tool.range != null) {
    items.push({ id: "range", label: `L${tool.range.start}-${tool.range.end}`, tone: "muted" });
  }
  if (tool.lines != null) {
    items.push({ id: "lines", label: t("tool.meta.lines", { count: tool.lines }), tone: "muted" });
  }
  if (tool.exitCode != null && tool.exitCode !== 0) {
    items.push({
      id: "exit",
      label: t("tool.meta.exit", { code: tool.exitCode }),
      tone: "negative",
    });
  }
  if (tool.durationMillis != null && tool.durationMillis >= 1000) {
    items.push({ id: "duration", label: fmtDuration(tool.durationMillis), tone: "muted" });
  }
  return items;
}

export function toolDiffStat(tool: ToolCall): { added: number; removed: number } | undefined {
  if (tool.status === "denied" || tool.status === "err") return undefined;
  const added = tool.added ?? 0;
  const removed = tool.removed ?? 0;
  if (tool.added == null && tool.removed == null) return undefined;
  if (added === 0 && removed === 0) return undefined;
  return { added, removed };
}

export function isReadOnlyTool(tool: ToolCall): boolean {
  return tool.safetyClass === "safe";
}

export function toolGroupNeedsAttention(tools: readonly ToolCall[]): boolean {
  return tools.some((tool) => tool.status === "running" || tool.status === "err");
}

const ACTIVITY_FAMILIES = ["read", "search", "lookup", "write", "run", "fetch"] as const;

type ActivityFamily = (typeof ACTIVITY_FAMILIES)[number];

function activityFamily(tool: ToolCall): ActivityFamily {
  if (tool.name === "read") return "read";
  if (tool.name === "lsp") return "lookup";
  if (tool.safetyClass === "write") return "write";
  if (tool.safetyClass === "exec") return "run";
  if (tool.safetyClass === "network") return "fetch";
  return "search";
}

export function summarizeActivity(t: Translate, tools: readonly ToolCall[]): string {
  const counts = new Map<ActivityFamily, number>();
  for (const tool of tools) {
    const family = activityFamily(tool);
    counts.set(family, (counts.get(family) ?? 0) + 1);
  }

  const parts: string[] = [];
  for (const family of ACTIVITY_FAMILIES) {
    const count = counts.get(family);
    if (count) parts.push(t(`tool.group.${family}`, { count }));
  }
  return parts.join(" · ");
}

function parseToolArgs(args: string): Record<string, unknown> | null {
  try {
    const parsed: unknown = JSON.parse(args || "{}");
    return parsed && typeof parsed === "object" && !Array.isArray(parsed)
      ? (parsed as Record<string, unknown>)
      : null;
  } catch {
    return null;
  }
}

function text(value: string | undefined): ToolDetail | undefined {
  return value === undefined || value === "" ? undefined : { kind: "machine", value };
}

function toolDetail(args: Record<string, unknown>): ToolDetail | undefined {
  for (const { key, kind } of TOOL_DETAIL_KEYS) {
    const value = args[key];
    if (value != null) return { kind, value: String(value) };
  }
  return undefined;
}

import { parseJSON } from "@frame/sdk/json";
import { type Translate } from "@/lib/i18n/index";
import { type ToolCall } from "@/features/agent/public/sessionView";
import { fmtDuration } from "@/lib/format";
import { toolVerbId } from "@/features/agent/domain/toolFamilies";
import { builtinToolName } from "@/features/agent/domain/toolCategory";
import { type BuiltinToolName } from "@/foundation/protocolValues.generated";
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

const TOOL_DETAIL_KEYS: ReadonlyArray<{ key: string; kind: ToolDetail["kind"] }> = [
  { key: "path", kind: "path" },
  { key: "query", kind: "machine" },
  { key: "pattern", kind: "machine" },
  { key: "url", kind: "machine" },
];

export function toolIntent(t: Translate, tool: ToolCall): ToolIntent {
  const labelKey = toolVerbId(tool.name);
  const tense = tool.status === "running" ? "doing" : tool.status === "ok" ? "done" : "action";
  // A tool outside the built-in vocabulary names itself; the host has no verb
  // for another owner's tool.
  const verb: ToolDetail =
    labelKey === undefined
      ? { kind: "machine", value: tool.name }
      : { kind: "prose", value: t(`tool.${tense}.${labelKey}`) };
  const command = text(tool.command);
  const described = command !== undefined && tool.fn !== command.value;
  const argument: ToolDetail | undefined =
    tool.fn === tool.name ? undefined : { kind: tool.fnKind ?? "machine", value: tool.fn };
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
    items.push({ id: "hits", label: t("tool.meta.matches", { count: tool.hits }), tone: "muted" });
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
    items.push({ id: "duration", label: fmtDuration(t, tool.durationMillis), tone: "muted" });
  }
  const offset = skillTextOffset(tool);
  if (offset !== undefined) {
    items.push({ id: "offset", label: t("tool.meta.fromByte", { offset }), tone: "muted" });
  }
  return items;
}

const SKILL_SNAPSHOT = /^[0-9a-f]{64}$/i;

// Only Skill text pages count bytes; list_skills' cursor offset indexes entries.
function skillTextOffset(tool: ToolCall): number | undefined {
  const name = builtinToolName(tool);
  if (name !== "load_skill" && name !== "read_skill_resource") return undefined;
  const cursor = parseToolArgs(tool.args)?.cursor;
  if (cursor === null || typeof cursor !== "object" || Array.isArray(cursor)) return undefined;
  const { offset, snapshot } = cursor as Record<string, unknown>;
  if (typeof offset !== "number" || !Number.isSafeInteger(offset) || offset <= 0) return undefined;
  return typeof snapshot === "string" && SKILL_SNAPSHOT.test(snapshot) ? offset : undefined;
}

export function isReadOnlyTool(tool: ToolCall): boolean {
  return tool.safetyClass === "safe";
}

export function toolGroupNeedsAttention(tools: readonly ToolCall[]): boolean {
  return tools.some((tool) => tool.status === "running" || tool.status === "err");
}

export interface ToolActivityCounts {
  running: number;
  issues: number;
  denied: number;
}

export function toolActivityStatus(tools: readonly ToolCall[]): ToolActivityCounts | null {
  const counts = { running: 0, issues: 0, denied: 0 };
  for (const tool of tools) {
    if (tool.status === "running") counts.running += 1;
    else if (tool.status === "denied") counts.denied += 1;
    else if (tool.status === "err" || (tool.exitCode != null && tool.exitCode !== 0))
      counts.issues += 1;
  }
  return counts.running + counts.issues + counts.denied > 0 ? counts : null;
}

const ACTIVITY_FAMILIES = ["read", "search", "write", "run", "fetch", "call"] as const;

type ActivityFamily = (typeof ACTIVITY_FAMILIES)[number];

// A family names what a built-in tool does. Safety classes only decide approval:
// Runtime classes plugin and MCP tools as exec so approval fails closed.
const BUILTIN_ACTIVITY: Partial<Record<BuiltinToolName, ActivityFamily>> = {
  read: "read",
  read_shell_output: "read",
  read_tool_result: "read",
  read_skill_resource: "read",
  load_skill: "read",
  list_skills: "read",
  grep: "search",
  glob: "search",
  search_tools: "search",
  shell: "run",
  stop_shell: "run",
  run_script: "run",
};

function activityFamily(tool: ToolCall): ActivityFamily {
  if (tool.safetyClass === "write") return "write";
  if (tool.safetyClass === "network") return "fetch";
  const builtin = builtinToolName(tool);
  return (builtin && BUILTIN_ACTIVITY[builtin]) ?? "call";
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
    const parsed: unknown = parseJSON(args || "{}");
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
  // Keep the requested position first without interpreting another owner's units.
  const { offset, snapshotId } = args;
  if (typeof snapshotId !== "string" || snapshotId === "") return undefined;
  if (typeof offset !== "number" || !Number.isSafeInteger(offset)) return undefined;
  return { kind: "machine", value: JSON.stringify({ offset, snapshotId }) };
}

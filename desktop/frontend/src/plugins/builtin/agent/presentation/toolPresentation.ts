import type { Translate } from "@/lib/i18n";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import type { ActivityShell } from "@/lib/activityShell";
import { fmtDuration } from "@/lib/format";

// Typed, not a bare string: a path truncates from the OTHER end, and a plain string leaves
// the renderer guessing — every path then loses its filename to an ellipsis.
export type ToolDetail = { kind: "path" | "text"; value: string };

export interface ToolIntent {
  /** For the file categories the projection puts the path HERE rather than in `detail`. */
  label: ToolDetail;
  detail?: ToolDetail;
}

export type ToolMetaTone = "success" | "negative" | "muted";

export interface ToolMetaItem {
  id: string;
  label: string;
  tone: ToolMetaTone;
}

// Every built-in tool's VERB. A row states the act before the thing acted on, so these are
// the label and the identifying argument is the detail — one glance answers "doing what, to
// what" without decoding a glyph. A tool the runtime added and this table has not heard of
// keeps its wire name as the thing, under a generic verb.
const TOOL_LABEL_KEYS = new Map([
  ["shell", "tool.label.shell"],
  ["read_shell_output", "tool.label.readShellOutput"],
  ["stop_shell", "tool.label.stopShell"],
  ["read", "tool.label.read"],
  ["edit", "tool.label.edit"],
  ["write", "tool.label.write"],
  ["apply_patch", "tool.label.applyPatch"],
  ["grep", "tool.label.grep"],
  ["glob", "tool.label.glob"],
  ["lsp", "tool.label.lsp"],
  ["web_search", "tool.label.webSearch"],
  ["web_fetch", "tool.label.webFetch"],
  ["http_request", "tool.label.httpRequest"],
  ["list_skills", "tool.label.listSkills"],
  ["load_skill", "tool.label.loadSkill"],
  ["read_skill_resource", "tool.label.readSkillResource"],
  ["propose_skill", "tool.label.proposeSkill"],
  ["delegate_task", "tool.label.delegateTask"],
  ["ask_user", "tool.label.askUser"],
  ["enter_plan_mode", "tool.label.enterPlanMode"],
  ["set_plan", "tool.label.setPlan"],
  ["exit_plan_mode", "tool.label.exitPlanMode"],
  ["search_memory", "tool.label.searchMemory"],
  ["search_conversations", "tool.label.searchConversations"],
  ["search_tools", "tool.label.searchTools"],
  ["read_tool_result", "tool.label.readToolResult"],
  ["list_schedules", "tool.label.listSchedules"],
  ["create_schedule", "tool.label.createSchedule"],
  ["delete_schedule", "tool.label.deleteSchedule"],
  ["create_goal", "tool.label.createGoal"],
  ["get_goal", "tool.label.getGoal"],
  ["report_goal_outcome", "tool.label.reportGoalOutcome"],
]);

// `path` is the runtime's own spelling, which ApprovalSubject reads too, so a rename cannot
// drift these apart silently.
const TOOL_DETAIL_KEYS: ReadonlyArray<{ key: string; kind: ToolDetail["kind"] }> = [
  { key: "path", kind: "path" },
  { key: "query", kind: "text" },
  { key: "pattern", kind: "text" },
  { key: "url", kind: "text" },
];

export function toolIntent(t: Translate, tool: ToolCall): ToolIntent {
  const labelKey = TOOL_LABEL_KEYS.get(tool.name);
  const verb: ToolDetail = { kind: "text", value: t(labelKey ?? "tool.label.generic") };
  const command = text(tool.command);
  // Prose outranks the verb; an argument does not. A shell call's `fn` is the model's own
  // account of what the run is FOR, which no table can restate — but everywhere else `fn` is
  // the identifying argument, and putting it in the title leaves the row never saying what
  // was done to it. `fn` restating the tool's own name means the projection found nothing.
  const described = command !== undefined && tool.fn !== command.value;
  const argument: ToolDetail | undefined =
    tool.fn === tool.name && labelKey !== undefined
      ? undefined
      : { kind: tool.fnKind ?? "text", value: tool.fn };
  const label = described ? { kind: "text" as const, value: tool.fn } : verb;
  const parsed = parseToolArgs(tool.args);
  const detail =
    command ??
    (described ? undefined : argument) ??
    text(tool.step) ??
    (parsed ? toolDetail(parsed) : undefined);
  return detail && detail.value === label.value ? { label } : { label, detail };
}

export function toolMetaItems(t: Translate, tool: ToolCall): ToolMetaItem[] {
  const items: ToolMetaItem[] = [];
  // Added/removed are NOT chips: a diffstat is one fact with two numbers. Ratios and line
  // spans stay notation — they read the same in every language.
  if (tool.progress != null) {
    items.push({
      id: "progress",
      label: `${tool.progress.done}/${tool.progress.total}`,
      tone: "muted",
    });
  }
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
  // Sub-second calls omitted.
  if (tool.durationMillis != null && tool.durationMillis >= 1000) {
    items.push({ id: "duration", label: fmtDuration(tool.durationMillis), tone: "muted" });
  }
  return items;
}

// Absent, not zeroed, when nothing was measured: a zero would draw a dash on the row.
export function toolDiffStat(tool: ToolCall): { added: number; removed: number } | undefined {
  // The counts come from the patch the call was GIVEN, so they survive a call that never
  // applied it. A refusal or a failure must not wear the size of a change that never landed.
  if (tool.status === "denied" || tool.status === "err") return undefined;
  const added = tool.added ?? 0;
  const removed = tool.removed ?? 0;
  if (tool.added == null && tool.removed == null) return undefined;
  if (added === 0 && removed === 0) return undefined;
  return { added, removed };
}

// The runtime's own answer, the same table the approval gate reads, so the row's weight and
// the gate's decision cannot disagree.
export function isReadOnlyTool(tool: ToolCall): boolean {
  return tool.safetyClass === "safe";
}

// Constant on purpose: an invocation stays on the work-narrative plane whatever its safety
// class or outcome.
export function toolActivityShell(_tool: ToolCall): ActivityShell {
  return "line";
}

export function toolGroupNeedsAttention(tools: readonly ToolCall[]): boolean {
  return tools.some((tool) => tool.status === "running" || tool.status === "err");
}

// Derived from the runtime's safety classes, so a tool added on the backend lands in the
// right family with no table here. Order is FIXED, not by count: a row that reorders as
// counts change has to be re-read every time.
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
  return value === undefined || value === "" ? undefined : { kind: "text", value };
}

function toolDetail(args: Record<string, unknown>): ToolDetail | undefined {
  for (const { key, kind } of TOOL_DETAIL_KEYS) {
    const value = args[key];
    if (value != null) return { kind, value: String(value) };
  }
  return undefined;
}

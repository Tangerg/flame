import type { Translate } from "@/lib/i18n";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import type { ActivityShell } from "@/lib/activityShell";
import { fmtDuration } from "@/lib/format";

// Typed rather than a bare string: a path truncates from the other end (see FilePath), and
// only this layer knows which of a call's arguments it read. A plain string leaves the
// renderer guessing and every path loses its filename to an ellipsis.
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

// A row's title is either one of these UI-chosen verbs or the runtime's own tool name,
// which is data and stays verbatim. That mix is why the translator arrives as an argument
// here instead of this returning a `labelKey`.
const TOOL_LABEL_KEYS = new Map([
  ["shell", "tool.label.shell"],
  ["read", "tool.label.read"],
  ["edit", "tool.label.edit"],
  ["write", "tool.label.write"],
  ["apply_patch", "tool.label.applyPatch"],
  ["grep", "tool.label.grep"],
  ["glob", "tool.label.glob"],
  ["lsp", "tool.label.lsp"],
  // No identifying argument to title with, so without an entry here these read as raw
  // snake_case names.
  ["enter_plan_mode", "tool.label.enterPlanMode"],
  ["set_plan", "tool.label.setPlan"],
  ["exit_plan_mode", "tool.label.exitPlanMode"],
  ["list_skills", "tool.label.listSkills"],
  ["list_schedules", "tool.label.listSchedules"],
  ["read_tool_result", "tool.label.readToolResult"],
]);

// `path` is the runtime's own spelling (ApprovalSubject reads the same field), so a rename
// cannot drift these apart without also breaking the approval rules.
const TOOL_DETAIL_KEYS: ReadonlyArray<{ key: string; kind: ToolDetail["kind"] }> = [
  { key: "path", kind: "path" },
  { key: "query", kind: "text" },
  { key: "pattern", kind: "text" },
  { key: "url", kind: "text" },
];

export function toolIntent(t: Translate, tool: ToolCall): ToolIntent {
  const parsed = parseToolArgs(tool.args);
  // Keyed on "the projection had nothing better than the tool's own name", NOT on `fn`
  // matching a table entry — the same thing by coincidence until a shell command happens
  // to be spelled `grep`.
  const labelKey = tool.fn === tool.name ? TOOL_LABEL_KEYS.get(tool.name) : undefined;
  const label: ToolDetail = labelKey
    ? { kind: "text", value: t(labelKey) }
    : { kind: tool.fnKind ?? "text", value: tool.fn };
  const detail = text(tool.command) ?? text(tool.step) ?? (parsed ? toolDetail(parsed) : undefined);
  // Where the title already IS the subject the second slot has nothing to say: `description`
  // is the tool's contract, not the wire's guarantee, so an absent one falls the title back
  // to the command and both slots print the same shell line at different widths.
  return detail && detail.value === label.value ? { label } : { label, detail };
}

export function toolMetaItems(t: Translate, tool: ToolCall): ToolMetaItem[] {
  const items: ToolMetaItem[] = [];
  // Added/removed are deliberately NOT chips — a diffstat is one fact with two numbers and
  // renders through `toolDiffStat`. Ratios and line spans stay as notation rather than
  // catalog entries: they read the same in every language.
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
  // Sub-second calls omitted: the number only earns its place once it answers "why did
  // that take so long".
  if (tool.durationMillis != null && tool.durationMillis >= 1000) {
    items.push({ id: "duration", label: fmtDuration(tool.durationMillis), tone: "muted" });
  }
  if (tool.status === "running") {
    items.push({ id: "live", label: t("tool.meta.live"), tone: "muted" });
  }
  return items;
}

// Absent rather than zeroed when nothing was measured: the atom draws a dash to hold a
// column, and a dash on a transcript row is a mark the reader has to stop and interpret.
export function toolDiffStat(tool: ToolCall): { added: number; removed: number } | undefined {
  const added = tool.added ?? 0;
  const removed = tool.removed ?? 0;
  if (tool.added == null && tool.removed == null) return undefined;
  if (added === 0 && removed === 0) return undefined;
  return { added, removed };
}

// The runtime's own answer, stamped on the Item — the same table the approval gate reads,
// so the row's weight and the gate's decision cannot disagree. A presentation-side list
// would duplicate protocol policy and go stale.
export function isReadOnlyTool(tool: ToolCall): boolean {
  return tool.safetyClass === "safe";
}

// Constant on purpose: an invocation stays on the work-narrative plane regardless of safety
// class or outcome, and only material results earn a surface inside the disclosed body.
// Keeping the boundary here stops new tools and lifecycle states from reintroducing a
// dashboard-like stack of tinted cards.
export function toolActivityShell(_tool: ToolCall): ActivityShell {
  return "line";
}

export function toolGroupNeedsAttention(tools: readonly ToolCall[]): boolean {
  return tools.some((tool) => tool.status === "running" || tool.status === "err");
}

// The runtime's own safety classes plus the two reads worth telling apart, so a tool added
// or renamed on the backend lands in the right family with no table here to update. Order
// is FIXED, not by count: a row that reorders as counts change must be re-read every time.
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

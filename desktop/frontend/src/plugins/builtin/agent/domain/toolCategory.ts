export type ToolCategory =
  "command" | "fileEdit" | "search" | "webSearch" | "read" | "subagent" | "generic";

const TOOL_CATEGORY = new Map<string, ToolCategory>([
  ["shell", "command"],
  ["apply_patch", "fileEdit"],
  ["grep", "search"],
  ["glob", "search"],
  ["web_search", "webSearch"],
  ["read", "read"],
  ["delegate_task", "subagent"],
]);

export function toolCategory(name: string): ToolCategory {
  return TOOL_CATEGORY.get(name) ?? "generic";
}

const QUESTION_TOOLS = new Set(["ask_user", "exit_plan_mode"]);
export function isQuestionTool(name: string): boolean {
  return QUESTION_TOOLS.has(name);
}

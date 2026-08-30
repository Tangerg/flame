// The agent context's own vocabulary: tool NAMES are the runtime's, and what they mean
// for presentation belongs here rather than in the kernel's contract types.

export type ToolCategory =
  | "command" // shell → { command, description } + { output, exitCode? }, or a plain-string ack when backgrounded
  | "fileEdit" // apply_patch → { patch } + { changes: AppliedChange[] }
  | "search" // grep / glob → { pattern } + { hits: SearchHit[] }
  | "webSearch" // web_search → { query } + { results: WebSearchResult[] }
  | "read" // read → { path, start_line?, max_lines? } + { content, start_line, … }
  | "subagent" // delegate_task → { summary, instructions } + a plain-string reply
  | "generic"; // MCP "<server>_<tool>" / anything unknown → JSON tree

const TOOL_CATEGORY: Record<string, ToolCategory> = {
  shell: "command",
  // The only built-in file mutation, and its result is a CALL-SCOPED receipt — not a
  // workspace diff.
  apply_patch: "fileEdit",
  grep: "search",
  glob: "search",
  web_search: "webSearch",
  read: "read",
  delegate_task: "subagent", // the runtime's delegation tool (spawns a child run, returns its reply)
};
// Everything else stays "generic" ON PURPOSE: labels, icons and previews key on the tool
// NAME, and the generic projection already passes their results through.

export function toolCategory(name: string): ToolCategory {
  return TOOL_CATEGORY[name] ?? "generic";
}

// These interrupt from inside their own call, so the runtime emits BOTH a toolCall Item
// (drained to `incomplete` when the turn parks) AND a question Item. The QuestionCard is
// the real representation; the tool row is a shadow that reads as a red ✗ through the
// incomplete→err mapping, so the renderer drops it whenever the question block is present.
const QUESTION_TOOLS = new Set(["ask_user", "exit_plan_mode"]);
export function isQuestionTool(name: string): boolean {
  return QUESTION_TOOLS.has(name);
}

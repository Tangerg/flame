const QUESTION_TOOLS = new Set(["ask_user", "exit_plan_mode"]);

export function isQuestionTool(name: string): boolean {
  return QUESTION_TOOLS.has(name);
}

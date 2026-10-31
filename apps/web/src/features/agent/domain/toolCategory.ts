import {
  BUILTIN_TOOL_NAMES,
  type BuiltinToolName,
  type ToolCategory,
} from "@/foundation/protocolValues.generated";

/** Only built-in tools carry a category; every other name is its owner's vocabulary. */
export function builtinToolName(tool: {
  name: string;
  category?: ToolCategory;
}): BuiltinToolName | undefined {
  return tool.category === undefined
    ? undefined
    : BUILTIN_TOOL_NAMES.find((name) => name === tool.name);
}

const QUESTION_TOOLS: ReadonlySet<BuiltinToolName> = new Set<BuiltinToolName>([
  "ask_user",
  "exit_plan_mode",
]);

export function isQuestionTool(tool: { name: string; category?: ToolCategory }): boolean {
  const name = builtinToolName(tool);
  return name !== undefined && QUESTION_TOOLS.has(name);
}

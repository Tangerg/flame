import { type ToolRef } from "@flame/runtime-client/wire";

// Families group Runtime built-in tools only; a plugin or MCP tool's name is
// its owner's vocabulary, so the host renders it generically.
type BuiltinToolName = Extract<ToolRef, { type: "builtIn" }>["name"];

interface ToolFamily {
  id: string;
  tools: readonly { name: BuiltinToolName; icon: string }[];
}

export const TOOL_FAMILIES = [
  {
    id: "shell",
    tools: [
      { name: "shell", icon: "terminal" },
      { name: "read_shell_output", icon: "scroll" },
      { name: "stop_shell", icon: "stop" },
    ],
  },
  {
    id: "files",
    tools: [
      { name: "read", icon: "file-text" },
      { name: "apply_patch", icon: "file-diff" },
    ],
  },
  {
    id: "search",
    tools: [
      { name: "grep", icon: "text-search" },
      { name: "glob", icon: "folder-search" },
      { name: "search_tools", icon: "tool-search" },
    ],
  },
  {
    id: "skills",
    tools: [
      { name: "list_skills", icon: "library" },
      { name: "load_skill", icon: "book-open" },
      { name: "read_skill_resource", icon: "file" },
      { name: "propose_skill", icon: "file-edit" },
    ],
  },
  {
    id: "delegation",
    tools: [
      { name: "delegate_task", icon: "users" },
      { name: "ask_user", icon: "question" },
    ],
  },
  {
    id: "plan",
    tools: [
      { name: "enter_plan_mode", icon: "workflow" },
      { name: "set_plan", icon: "list-checks" },
      { name: "exit_plan_mode", icon: "flag" },
    ],
  },
  {
    id: "recall",
    tools: [{ name: "read_tool_result", icon: "file-output" }],
  },
  {
    id: "goals",
    tools: [
      { name: "create_goal", icon: "target" },
      { name: "get_goal", icon: "clipboard-list" },
      { name: "report_goal_outcome", icon: "clipboard-check" },
    ],
  },
] as const satisfies readonly ToolFamily[];

type ToolFamilyId = (typeof TOOL_FAMILIES)[number]["id"];

export function toolFamilyNames(id: ToolFamilyId): readonly string[] {
  return TOOL_FAMILIES.find((family) => family.id === id)!.tools.map((tool) => tool.name);
}

export function toolFamilyId(name: string): string | undefined {
  return FAMILY_BY_TOOL.get(name);
}

export function toolVerbId(name: string): string | undefined {
  if (!FAMILY_BY_TOOL.has(name)) return undefined;
  return name.replace(/_([a-z])/g, (_, initial: string) => initial.toUpperCase());
}

const FAMILY_BY_TOOL = new Map<string, string>(
  TOOL_FAMILIES.flatMap((family) => family.tools.map((tool) => [tool.name, family.id] as const)),
);

export const TOOL_ICON_BY_NAME: Readonly<Record<string, string>> = Object.fromEntries(
  TOOL_FAMILIES.flatMap((family) => family.tools.map((tool) => [tool.name, tool.icon])),
);

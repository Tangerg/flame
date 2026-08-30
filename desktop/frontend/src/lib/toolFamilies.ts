/**
 * In lib because the transcript needs the glyph and the catalog needs the family, and those
 * two contexts may not import each other.
 *
 * ONE GLYPH PER TOOL, held by a test. Colour is deliberately NOT part of the
 * differentiation: tone means STATE, so spending it on identity leaves a failed read and a
 * successful one equally alarming. This is the built-in VOCABULARY, not the live inventory —
 * `tools.list` stays the authority for what is exposed.
 */
export interface ToolFamily {
  /** i18n key suffix: `tools.family.<id>`. */
  id: string;
  tools: readonly { name: string; icon: string }[];
}

export const TOOL_FAMILIES: readonly ToolFamily[] = [
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
      { name: "read", icon: "eye" },
      { name: "apply_patch", icon: "replace" },
    ],
  },
  {
    id: "search",
    tools: [
      { name: "grep", icon: "text-search" },
      { name: "glob", icon: "folder-search" },
      { name: "lsp", icon: "code" },
    ],
  },
  {
    id: "network",
    tools: [
      { name: "web_search", icon: "globe" },
      { name: "web_fetch", icon: "download" },
      { name: "http_request", icon: "webhook" },
    ],
  },
  {
    id: "skills",
    tools: [
      { name: "list_skills", icon: "library" },
      { name: "load_skill", icon: "book-open" },
      { name: "read_skill_resource", icon: "paperclip" },
      { name: "propose_skill", icon: "sparkle" },
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
      { name: "enter_plan_mode", icon: "map" },
      { name: "set_plan", icon: "list-checks" },
      { name: "exit_plan_mode", icon: "flag" },
    ],
  },
  {
    id: "recall",
    tools: [
      { name: "search_memory", icon: "brain" },
      { name: "search_conversations", icon: "history" },
      { name: "search_tools", icon: "package-search" },
      { name: "read_tool_result", icon: "archive" },
    ],
  },
  {
    id: "schedules",
    tools: [
      { name: "list_schedules", icon: "clock" },
      { name: "create_schedule", icon: "calendar-plus" },
      { name: "delete_schedule", icon: "calendar-x" },
    ],
  },
  {
    id: "goals",
    tools: [
      { name: "create_goal", icon: "target" },
      { name: "get_goal", icon: "crosshair" },
      { name: "report_goal_outcome", icon: "clipboard-check" },
    ],
  },
];

/** `undefined` for a tool this table has never heard of. Nothing here FABRICATES a family;
 *  the catalog gives unplaced tools their own heading. */
export function toolFamilyId(name: string): string | undefined {
  return FAMILY_BY_TOOL.get(name);
}

const FAMILY_BY_TOOL = new Map<string, string>(
  TOOL_FAMILIES.flatMap((family) => family.tools.map((tool) => [tool.name, family.id] as const)),
);

export const TOOL_ICON_BY_NAME: Readonly<Record<string, string>> = Object.fromEntries(
  TOOL_FAMILIES.flatMap((family) => family.tools.map((tool) => [tool.name, tool.icon])),
);

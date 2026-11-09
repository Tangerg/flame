export const VISUAL_DOCK_WIDTH_RATIO = 0.5;

export const VISUAL_WORKSPACE_VIEWPORT = { width: 1472, height: 900 } as const;

export const VISUAL_REVIEW_DOCK_WIDTH_RATIO = 0.36;

export const VISUAL_REVIEW_VIEWPORT = { width: 1800, height: 1000 } as const;

export const VISUAL_SETTINGS_PANES = [
  "appearance",
  "brand",
  "personalization",
  "providers",
  "approvals",
  "mcp-servers",
  "hooks",
  "plugins",
  "connection",
  "shortcuts",
] as const;

export type VisualSettingsPane = (typeof VISUAL_SETTINGS_PANES)[number];

export function isVisualSettingsPane(value: string | null): value is VisualSettingsPane {
  return VISUAL_SETTINGS_PANES.includes(value as VisualSettingsPane);
}

export const VISUAL_WORKSPACE_STATES = [
  "dock-light",
  "dock-review",
  "dock-trajectory",
  "dock-runs",
  "dock-subagents",
  "dock-diagnostics",
  "dock-diagnostics-recorded",
  "dock-files",
  "dock-skills",
  "dock-skills-catalog",
  "dock-agent-memory",
  "dock-schedules",
  "dock-schedules-models",
  "dock-usage",
  "dock-feature-off",
  "dock-file",
  "dock-file-error",
  "dock-file-unsupported",
  "dock-empty",
  "dock-catalog",
  "dock-loading",
  "dock-error",
  "full-view",
  "settings",
] as const;

export type VisualWorkspaceState = (typeof VISUAL_WORKSPACE_STATES)[number];

export type VisualWorkspaceTheme = "light" | "dark";

export function isVisualWorkspaceState(value: string | null): value is VisualWorkspaceState {
  return VISUAL_WORKSPACE_STATES.includes(value as VisualWorkspaceState);
}

export const DOCK_VIEW_BY_STATE: Partial<Record<VisualWorkspaceState, string>> = {
  "dock-light": "file",
  "dock-review": "package:visual:diff",
  "dock-empty": "package:visual:diff",
  "dock-loading": "package:visual:diff",
  "dock-error": "package:visual:diff",
  "dock-trajectory": "package:visual:trajectory",
  "dock-runs": "package:visual:trajectory",
  "dock-subagents": "subagents",
  "dock-diagnostics": "package:visual:diagnostics",
  "dock-diagnostics-recorded": "package:visual:diagnostics",
  "dock-files": "file",
  "dock-skills": "package:visual:skills",
  "dock-skills-catalog": "package:visual:skills",
  "dock-agent-memory": "package:visual:memory",
  "dock-schedules": "package:visual:schedules",
  "dock-schedules-models": "package:visual:schedules",
  "dock-usage": "package:visual:usage",
  "dock-feature-off": "package:visual:skills",
  "dock-file": "file",
  "dock-file-error": "file",
  "dock-file-unsupported": "file",
};

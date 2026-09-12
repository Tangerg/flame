export const VISUAL_DOCK_WIDTH_RATIO = 0.5;

export const VISUAL_WORKSPACE_VIEWPORT = { width: 1472, height: 900 } as const;

export const VISUAL_REVIEW_DOCK_WIDTH_RATIO = 0.36;

export const VISUAL_REVIEW_VIEWPORT = { width: 1800, height: 1000 } as const;

export const VISUAL_SETTINGS_PANES = [
  "appearance",
  "personalization",
  "providers",
  "approvals",
  "mcp-servers",
  "hooks",
  "schedules",
  "plugins",
  "usage",
  "connection",
  "brand-icons",
  "shortcuts",
] as const;

export type VisualSettingsPane = (typeof VISUAL_SETTINGS_PANES)[number];

export function isVisualSettingsPane(value: string | null): value is VisualSettingsPane {
  return VISUAL_SETTINGS_PANES.includes(value as VisualSettingsPane);
}

export const VISUAL_WORKSPACE_STATES = [
  "dock-light",
  "dock-review",
  "dock-inbox",
  "dock-timeline",
  "dock-runs",
  "dock-explorer",
  "dock-search",
  "dock-skill-proposals",
  "dock-skill-library",
  "dock-recipes",
  "dock-agent-docs",
  "dock-skills",
  "dock-knowledge",
  "dock-agent-memory",
  "dock-feature-off",
  "dock-notifications",
  "dock-tools",
  "dock-file",
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

/**
 * The dock width every workspace golden is shot at, and the one the resize specs
 * treat as "a preference the user set".
 *
 * Chosen against three constraints rather than picked: wide enough that the review
 * split renders side by side (>= the 560 at which the navigator withdraws — the
 * seeded 520 put every golden on the collapsed side of that line, so the file
 * navigator appeared in no screenshot at all), narrow enough to survive at the
 * canonical 1472px viewport (`maxDockWidth` = 576 there) so the clamp specs can
 * prove a preference is kept,
 * and wide enough to become unavailable at 1120 so they can prove the preference
 * is not overwritten when the whole dock folds.
 */
export const VISUAL_DOCK_WIDTH_RATIO = 0.5;

/** The smallest canonical fixture viewport that preserves both seeded columns. */
export const VISUAL_WORKSPACE_VIEWPORT = { width: 1472, height: 900 } as const;

/**
 * What the review golden is shot at, and it is deliberately not the width above.
 *
 * The diff view is the only one that splits, and its split has a hard floor (720 —
 * see the container query in globals.css). Shooting it at the general width put
 * every review golden on the collapsed side of that floor, so the navigator's
 * side-by-side geometry — a whole track, its seam, and its filter strip — was
 * covered by nothing. This is the narrowest dock at which that geometry exists.
 */
export const VISUAL_REVIEW_DOCK_WIDTH_RATIO = 0.36;

/** The viewport the review golden needs for `maxDockWidth` to allow the width above. */
export const VISUAL_REVIEW_VIEWPORT = { width: 1800, height: 1000 } as const;

/**
 * The settings panes a fixture can open. Only `appearance` was ever shown — the settings
 * state hard-coded it — so eleven panes shipped with no frame and no accessibility audit.
 */
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
  "dock-stats",
  "dock-timeline",
  "dock-runs",
  "dock-explorer",
  "dock-terminal",
  "dock-search",
  "dock-files",
  "dock-skill-proposals",
  "dock-skill-library",
  "dock-recipes",
  "dock-agent-docs",
  "dock-skills",
  "dock-knowledge",
  "dock-agent-memory",
  "dock-feature-off",
  "dock-run-summary",
  "dock-notifications",
  "dock-tools",
  "dock-file",
  "dock-empty",
  "dock-catalog",
  "dock-loading",
  "dock-error",
  "settings",
] as const;

export type VisualWorkspaceState = (typeof VISUAL_WORKSPACE_STATES)[number];
export type VisualWorkspaceTheme = "light" | "dark";

export function isVisualWorkspaceState(value: string | null): value is VisualWorkspaceState {
  return VISUAL_WORKSPACE_STATES.includes(value as VisualWorkspaceState);
}

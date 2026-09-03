export const VISUAL_WORK_INDEX_STATES = ["populated", "empty", "loading", "error"] as const;

export type VisualWorkIndexState = (typeof VISUAL_WORK_INDEX_STATES)[number];
export type VisualShellTheme = "light" | "dark";

/** The two search overlays, which nothing photographed: both are `SearchOverlay`, and the
 *  atom owns focus handoff, so a frame of each is the only thing that can catch its chrome. */
export const VISUAL_SHELL_OVERLAYS = ["finder", "commands"] as const;

export type VisualShellOverlay = (typeof VISUAL_SHELL_OVERLAYS)[number];

export function isVisualShellOverlay(value: string | null): value is VisualShellOverlay {
  return VISUAL_SHELL_OVERLAYS.includes(value as VisualShellOverlay);
}

import type { ComponentType } from "react";

export interface SettingsPaneSpec {
  id: string;
  /** A catalog key resolved at RENDER time, not a literal. */
  label: string;
  /** A catalog key, like `label`. */
  description?: string;
  icon?: string;
  /** Lower comes first. Built-ins use 0..99; plugins ≥ 100. */
  order?: number;
  /** An omitted or unknown group falls into the trailing group, so nothing is dropped. */
  group?: string;
  component: ComponentType;
}

/**
 * Unlike `LayoutSlotSpec`, a workspace view does not pick its position — the user does. The
 * kernel needs only `id` and the component; everything else is a hint.
 */
export type ContextDockDestinationScope = "workspace" | "session" | "run";

export interface WorkspaceViewSpec {
  /** The layout PERSISTENCE key: renaming one strands a saved layout. */
  id: string;
  /** A catalog key resolved where the tab renders. A key the catalog does not have renders
   *  as itself, which is how a file view passes a filename.
   *  (See `CommandSpec.label`: a contribution outlives the moment it is made, and
   *  nothing re-registers on a language switch.) */
  title: string;
  icon?: string;
  /** A COMPONENT, not a value: only the view knows where its number comes from, and the
   *  tab strip must not subscribe to every view's data just to label it. A few characters
   *  at most — it renders inside a 28px tab beside a truncating title. */
  badge?: ComponentType;
  /** Lower comes first. */
  order?: number;
  /** Where the view goes, and the whole of it: a scope names the group the dock's add-panel
   *  menu files it under, and its ABSENCE says the view takes the content card instead, the
   *  way settings does. Declared here rather than in a list beside the registry, because the
   *  two could disagree — and did: a view left out of that list had no way in at all. */
  dock?: ContextDockDestinationScope;
  component: ComponentType;
}

/**
 * Most regions are conceptually singletons, but multiple contributions are allowed so
 * overlays can stack without forking the kernel. The component receives NO props — slot
 * consumers read stores and query hooks directly, so the kernel threads nothing down.
 */
export interface LayoutSlotSpec {
  /** Multiple registrations to the same slot dedupe on this. */
  id: string;
  /** Lower comes first. Built-ins use 0..99; plugins ≥ 100. */
  order?: number;
  className?: string;
  component: ComponentType;
}

/** The router is rebuilt from the registry at AppRouter mount, so additions take effect on
 *  the next reload or an explicit `rebuildRouter()`. */
export interface RouteSpec {
  id: string;
  path: string;
  component: ComponentType;
  /** Listing order only; does not affect matching. */
  order?: number;
}

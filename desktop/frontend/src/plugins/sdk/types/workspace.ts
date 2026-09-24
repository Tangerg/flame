import type { ComponentType } from "react";

export interface SettingsPaneSpec {
  id: string;
  label: string;
  description?: string;
  icon?: string;
  order?: number;
  group?: string;
  keywords?: readonly string[];
  component: ComponentType;
}

export type ContextDockDestinationScope = "workspace" | "session" | "run";

export interface WorkspaceViewSpec {
  id: string;
  title: string;
  icon?: string;
  badge?: ComponentType;
  order?: number;
  dock?: ContextDockDestinationScope;
  component: ComponentType;
}

export interface LayoutSlotSpec {
  id: string;
  order?: number;
  className?: string;
  component: ComponentType;
}

export interface RouteSpec {
  id: string;
  path: string;
  component: ComponentType;
  order?: number;
}

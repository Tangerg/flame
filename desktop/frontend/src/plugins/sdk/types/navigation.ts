import type { ComponentType } from "react";

export type WorkIndexItemScope = "global" | "session";
export type WorkIndexItemVariant = "expanded" | "rail";

/**
 * The Work Index is the agent's work index, NOT a generic feature menu: an item must declare
 * whether it is app-global or tied to the session list, and which sidebar variant it
 * renders in.
 */
export interface WorkIndexItemSpec {
  id: string;
  scope: WorkIndexItemScope;
  variant: WorkIndexItemVariant;
  order?: number;
  component: ComponentType;
}

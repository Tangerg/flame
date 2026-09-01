import type { ComponentType } from "react";

/** The agent's work index, NOT a generic feature menu: one ordered column of sections. */
export interface WorkIndexItemSpec {
  id: string;
  order?: number;
  component: ComponentType;
}

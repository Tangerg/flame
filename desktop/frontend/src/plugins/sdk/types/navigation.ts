import type { ComponentType } from "react";

export interface WorkIndexItemSpec {
  id: string;
  order?: number;
  component: ComponentType;
}

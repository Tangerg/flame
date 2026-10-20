import { type ComponentType } from "react";
import { defineExtensionPoint } from "@/extensions/contracts";
export interface RouteSpec {
  id: string;
  path: string;
  component: ComponentType;
  order?: number;
}

export const ROUTE = defineExtensionPoint<RouteSpec>({
  id: "flame.route",
  keying: "single",
});

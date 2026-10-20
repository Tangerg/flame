import { type ComponentType } from "react";
import { type CapturedSession } from "@flame/component-sdk";
import { defineExtensionPoint } from "@/extensions/contracts";
export interface SessionActionSpec {
  id: string;
  title: string;
  component: ComponentType<{ session: CapturedSession }>;
}

export const SESSION_ACTION = defineExtensionPoint<SessionActionSpec>({
  id: "flame.session.action",
  keying: "single",
});

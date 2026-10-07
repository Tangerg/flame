import type { ApprovalDecision } from "@flame/runtime-contract/wire";

export type { ApprovalDecision };

export type ApprovalMode = "safe" | "balanced" | "yolo";
export type RememberScope = "session" | "project" | "global";

export interface InterruptRef {
  sessionId: string;
  rootRunId: string;
  itemId: string;
}

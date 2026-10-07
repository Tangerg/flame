import type { ApprovalDecision, ApprovalMode } from "@flame/runtime-contract/wire";

export type { ApprovalDecision, ApprovalMode };

export type RememberScope = "session" | "project" | "global";

export interface InterruptRef {
  sessionId: string;
  rootRunId: string;
  itemId: string;
}

export type ApprovalDecision = "approved" | "declined";

export type ApprovalMode = "safe" | "balanced" | "yolo";
export type RememberScope = "session" | "project" | "global";

export interface InterruptRef {
  sessionId: string;
  rootRunId: string;
  itemId: string;
}

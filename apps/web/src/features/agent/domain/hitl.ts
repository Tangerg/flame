export type ApprovalDecision = "approve" | "deny";

export type ApprovalGate = "pass" | "prompt" | "deny";

export type ApprovalMode = "safe" | "balanced" | "yolo";

export interface ApprovalModePolicy {
  exec: ApprovalGate;
  mode: ApprovalMode;
  network: ApprovalGate;
  write: ApprovalGate;
}

export interface ApprovalModeResult {
  mode: ApprovalMode;
  modes: ApprovalModePolicy[];
}

export type RememberScope = "session" | "project" | "global";

export type InterruptResumePayload =
  | {
      type: "approval";
      decision: ApprovalDecision;
      editedArgs?: Record<string, unknown>;
      reason?: string;
      remember?: { scope: RememberScope };
    }
  | { type: "answer"; answers: string[][] };

export interface InterruptResumeInput {
  itemId: string;
  response: InterruptResumePayload;
}

export interface InterruptRef {
  sessionId: string;
  rootRunId: string;
  itemId: string;
}

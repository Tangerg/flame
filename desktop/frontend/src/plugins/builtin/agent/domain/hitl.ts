import type {
  ApprovalDecision,
  ApprovalGate,
  ApprovalMode,
  ApprovalModePolicy,
  ApprovalModeResult,
} from "@flame/runtime-contract/wire";

export type {
  ApprovalDecision,
  ApprovalGate,
  ApprovalMode,
  ApprovalModePolicy,
  ApprovalModeResult,
};

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

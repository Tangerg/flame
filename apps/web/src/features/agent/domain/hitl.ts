import {
  type ApprovalDecision,
  type ApprovalGate,
  type ApprovalMode,
  type RememberScopeKind as RememberScope,
} from "@flame/runtime-client/wire";
export type { ApprovalDecision, ApprovalGate, ApprovalMode, RememberScope };
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

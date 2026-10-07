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

export interface InterruptRef {
  sessionId: string;
  rootRunId: string;
  itemId: string;
}

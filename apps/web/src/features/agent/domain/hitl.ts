import {
  type ApprovalDecision,
  type ApprovalGate,
  type ApprovalMode,
  type BuiltinToolName,
  type RememberScopeKind as RememberScope,
} from "@/foundation/protocolValues.generated";
export type { ApprovalDecision, ApprovalGate, ApprovalMode, RememberScope };

/** What a remembered approval matches within one Tool. */
export type ApprovalSubject =
  { type: "all" } | { type: "exact"; value: string } | { type: "glob"; value: string };

/** The Tool a remembered approval belongs to. */
export type ApprovalToolRef =
  | { type: "builtIn"; name: BuiltinToolName }
  | {
      type: "mcp";
      name: string;
      server: {
        name: string;
        origin: { type: "user" } | { type: "installation"; installationId: string };
      };
    };
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

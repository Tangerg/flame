import { type ApprovalRuleDecision } from "@/foundation/protocolValues.generated";
import { type MCPServerID } from "@/features/mcp/public/serverIdentity";
type ApprovalSubject =
  { type: "all" } | { type: "exact"; value: string } | { type: "glob"; value: string };
type ToolRef =
  | { type: "builtIn"; name: string }
  | { type: "mcp"; name: string; server: MCPServerID }
  | { type: "a2a"; endpoint: string };

import { createDataQuery, createParameterizedDataQuery } from "@/extensions/dataQuery";
import { type ApprovalModeResult, type RememberScope } from "@/features/agent/domain/hitl";
export interface ApprovalRulesQuery {
  sessionId?: string;
}

export interface ApprovalRuleSummary {
  id: string;
  scope: RememberScope;
  tool: ToolRef;
  modelName: string;
  stale: boolean;
  subject: ApprovalSubject;
  dir?: string;
  decision: ApprovalRuleDecision;
}

export const APPROVAL_MODE_KEY = "approval-mode";

export const APPROVAL_RULES_KEY = "approval-rules";

export const useApprovalMode = createDataQuery<ApprovalModeResult>(APPROVAL_MODE_KEY);

export const useApprovalRules = createParameterizedDataQuery<
  ApprovalRulesQuery,
  ApprovalRuleSummary[]
>(APPROVAL_RULES_KEY);

import type { ApprovalRuleDecision, ApprovalSubject, ToolRef } from "@flame/runtime-contract/wire";
import { createDataQuery, createParameterizedDataQuery } from "@/plugins/sdk";
import type { ApprovalModeResult, RememberScope } from "../domain/hitl";

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

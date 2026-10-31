import { type ApprovalRuleDecision } from "@/foundation/protocolValues.generated";

import { createDataQuery, createParameterizedDataQuery } from "@/extensions/dataQuery";
import {
  type ApprovalModeResult,
  type ApprovalSubject,
  type ApprovalToolRef,
  type RememberScope,
} from "@/features/agent/domain/hitl";
export interface ApprovalRulesQuery {
  sessionId?: string;
}

export interface ApprovalRuleSummary {
  id: string;
  scope: RememberScope;
  tool: ApprovalToolRef;
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

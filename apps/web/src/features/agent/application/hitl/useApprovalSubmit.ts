import { useCallback } from "react";
import { type ApprovalDecision, type RememberScope } from "@/features/agent/domain/hitl";
import { useInterruptResume } from "@/features/agent/application/hitl/useInterruptResume";
import { useAgentDependencies } from "@/features/agent/application/agentContext";
import { type ApprovalActions } from "@/features/agent/application/hitl/approvalActions";
export interface ApprovalSubmitOptions {
  editedArgs?: Record<string, unknown>;
  rememberScope?: RememberScope;
}

interface ApprovalSubmit {
  submit: (decision: ApprovalDecision, opts?: ApprovalSubmitOptions) => void;
  pending: ApprovalDecision | null;
  submitted: boolean;
  registerActions: (actions: ApprovalActions) => () => void;
}

export function useApprovalSubmit(
  sessionId: string,
  rootRunId?: string,
  itemId?: string,
): ApprovalSubmit {
  const { pending, submitted, resume } = useInterruptResume<ApprovalDecision>(
    sessionId,
    rootRunId,
    itemId,
  );

  const submit = useCallback(
    (decision: ApprovalDecision, opts?: ApprovalSubmitOptions) => {
      resume(decision, {
        type: "approval",
        decision,
        ...(opts?.editedArgs ? { editedArgs: opts.editedArgs } : {}),
        ...(opts?.rememberScope ? { remember: { scope: opts.rememberScope } } : {}),
      });
    },
    [resume],
  );

  const { approvalActions } = useAgentDependencies();
  const registerActions = useCallback(
    (actions: ApprovalActions) => {
      if (!rootRunId || !itemId) return () => undefined;
      return approvalActions.register({ sessionId, rootRunId, itemId }, actions);
    },
    [approvalActions, itemId, rootRunId, sessionId],
  );

  return { submit, pending, submitted, registerActions };
}

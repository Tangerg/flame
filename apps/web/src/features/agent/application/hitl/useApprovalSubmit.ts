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
  available: boolean;
  responseStaged: boolean;
  registerActions: (actions: ApprovalActions) => () => void;
}

export function useApprovalSubmit(
  sessionId: string,
  rootRunId?: string,
  itemId?: string,
): ApprovalSubmit {
  const { state, available, submit: respond } = useInterruptResume(sessionId, rootRunId, itemId);

  const submit = useCallback(
    (decision: ApprovalDecision, opts?: ApprovalSubmitOptions) => {
      respond({
        type: "approval",
        decision,
        ...(opts?.editedArgs ? { editedArgs: opts.editedArgs } : {}),
        ...(opts?.rememberScope ? { remember: { scope: opts.rememberScope } } : {}),
      });
    },
    [respond],
  );

  const { approvalActions } = useAgentDependencies();
  const registerActions = useCallback(
    (actions: ApprovalActions) => {
      if (!rootRunId || !itemId) return () => undefined;
      return approvalActions.register({ sessionId, rootRunId, itemId }, actions);
    },
    [approvalActions, itemId, rootRunId, sessionId],
  );

  return { submit, available, responseStaged: state !== "unsubmitted", registerActions };
}

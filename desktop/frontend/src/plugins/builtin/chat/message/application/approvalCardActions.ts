import { useCallback, useEffect } from "react";
import {
  useApprovalSubmit,
  type ApprovalSubmitOptions,
  type RememberScope,
} from "@/plugins/builtin/agent/public/hitl";
import { canSubmitApproval } from "@/plugins/builtin/agent/public/messagePresentation";

export interface ApprovalArgsCommitter {
  commit: () => Record<string, unknown> | undefined | null;
}

export interface ApprovalCardActionState {
  disabled: boolean;
  approve: (rememberScope?: RememberScope) => void;
  decline: () => void;
}

export function approvalSubmitOptions({
  editedArgs,
  rememberScope,
}: {
  editedArgs?: Record<string, unknown>;
  rememberScope?: RememberScope;
}): ApprovalSubmitOptions | undefined {
  if (editedArgs === undefined && rememberScope === undefined) return undefined;
  return {
    ...(editedArgs !== undefined ? { editedArgs } : {}),
    ...(rememberScope !== undefined ? { rememberScope } : {}),
  };
}

export function canRegisterApprovalActions({
  resumeRunId,
  itemId,
  runtimeAvailable = true,
}: {
  resumeRunId?: string;
  itemId?: string;
  runtimeAvailable?: boolean;
}): boolean {
  return Boolean(runtimeAvailable && resumeRunId && itemId);
}

export function useApprovalCardActions({
  resumeRunId,
  itemId,
  argsEditor,
  runtimeAvailable,
}: {
  resumeRunId?: string;
  itemId?: string;
  argsEditor?: ApprovalArgsCommitter;
  runtimeAvailable: boolean;
}): ApprovalCardActionState {
  const { submit, pending, registerActions } = useApprovalSubmit(resumeRunId, itemId);

  const approve = useCallback(
    (rememberScope?: RememberScope) => {
      const editedArgs = argsEditor?.commit();
      if (editedArgs === null) return;
      submit("approve", approvalSubmitOptions({ editedArgs, rememberScope }));
    },
    [argsEditor, submit],
  );

  const decline = useCallback(() => {
    submit("deny");
  }, [submit]);

  const registerable = canRegisterApprovalActions({ resumeRunId, itemId, runtimeAvailable });
  useEffect(() => {
    if (!registerable) return;
    return registerActions({
      approve: () => approve(),
      decline,
    });
  }, [approve, decline, registerable, registerActions]);

  return {
    disabled: !runtimeAvailable || !canSubmitApproval({ resumeRunId, itemId, pending }),
    approve,
    decline,
  };
}

import type { ApprovalDecision } from "../domain/hitl";

export function canSubmitApproval({
  resumeRunId,
  itemId,
  pending,
}: {
  resumeRunId?: string;
  itemId?: string;
  pending: ApprovalDecision | null;
}): boolean {
  return Boolean(resumeRunId && itemId && pending === null);
}

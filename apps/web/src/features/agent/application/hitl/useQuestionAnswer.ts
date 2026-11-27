import { useCallback } from "react";
import { useInterruptResume } from "@/features/agent/application/hitl/useInterruptResume";
type QuestionAnswers = string[][];

interface QuestionAnswerSubmit {
  submit: (answers: QuestionAnswers) => void;
  pending: boolean;
  available: boolean;
}

export function useQuestionAnswer(
  sessionId: string,
  runId?: string,
  itemId?: string,
): QuestionAnswerSubmit {
  const { state, available, submit: respond } = useInterruptResume(sessionId, runId, itemId);

  const submit = useCallback(
    (answers: QuestionAnswers) => {
      respond({ type: "answer", answers });
    },
    [respond],
  );

  return { submit, pending: state === "pending", available };
}

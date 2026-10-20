import { useCallback } from "react";
import { useInterruptResume } from "@/features/agent/application/hitl/useInterruptResume";
type QuestionAnswers = string[][];

export interface QuestionAnswerSubmit {
  submit: (answers: QuestionAnswers) => void;
  pending: boolean;
}

export function useQuestionAnswer(runId?: string, itemId?: string): QuestionAnswerSubmit {
  const { pending, resume } = useInterruptResume<true>(runId, itemId);

  const submit = useCallback(
    (answers: QuestionAnswers) => {
      resume(true, { type: "answer", answers });
    },
    [resume],
  );

  return { submit, pending: pending !== null };
}

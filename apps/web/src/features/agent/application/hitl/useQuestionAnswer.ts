import { useCallback } from "react";
import { useInterruptResume } from "@/features/agent/application/hitl/useInterruptResume";
type QuestionAnswers = string[][];

export interface QuestionAnswerSubmit {
  submit: (answers: QuestionAnswers) => void;
  pending: boolean;
  submitted: boolean;
}

export function useQuestionAnswer(
  sessionId: string,
  runId?: string,
  itemId?: string,
): QuestionAnswerSubmit {
  const { pending, submitted, resume } = useInterruptResume<true>(sessionId, runId, itemId);

  const submit = useCallback(
    (answers: QuestionAnswers) => {
      resume(true, { type: "answer", answers });
    },
    [resume],
  );

  return { submit, pending: pending !== null, submitted };
}

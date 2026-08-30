import { useCallback } from "react";
import { useInterruptResume } from "./useInterruptResume";

// Answers preserve `Question.fields` order and every field always contributes one values
// array — already the wire shape, so nothing is normalized at the boundary (API.md §6).

export type QuestionAnswers = string[][];

export interface QuestionAnswerSubmit {
  submit: (answers: QuestionAnswers) => void;
  pending: boolean;
}

export function useQuestionAnswer(runId?: string, itemId?: string): QuestionAnswerSubmit {
  const { pending, resume } = useInterruptResume<true>(runId, itemId);

  const submit = useCallback(
    (answers: QuestionAnswers) => {
      // The local settle removes interaction latency after the Runtime accepted
      // the claim. Durable refresh/replay then replaces it with the same
      // authoritative Question.answers projection.
      resume(true, { type: "answer", answers }, { answered: true, answers });
    },
    [resume],
  );

  return { submit, pending: pending !== null };
}

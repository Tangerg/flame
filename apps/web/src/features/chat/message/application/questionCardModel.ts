import { useCallback, useMemo } from "react";
import {
  type BlockStatus,
  type ContentBlock,
  type QuestionItem,
} from "@/features/agent/public/contentBlock";
import { type TranscriptRow } from "@/features/agent/public/conversation";
import { useQuestionAnswer } from "@/features/agent/public/hitl";
import {
  canSubmitQuestion,
  questionDraftAnswers,
  questionDraftComplete,
  questionSettled,
  type QuestionAnswers,
  type QuestionDraft,
} from "@/features/agent/public/messagePresentation";
export interface QuestionCardSettledView {
  settled: boolean;
  answers?: QuestionAnswers;
}

type QuestionBlock = Extract<ContentBlock, { kind: "question" }>;

export interface PendingQuestionRequest {
  block: QuestionBlock;
  resumeRunId: string;
  itemId: string;
}

export function pendingQuestionRequest(
  rows: readonly TranscriptRow[],
): PendingQuestionRequest | null {
  for (let rowIndex = rows.length - 1; rowIndex >= 0; rowIndex -= 1) {
    const { message, facts } = rows[rowIndex]!;
    for (let blockIndex = message.blocks.length - 1; blockIndex >= 0; blockIndex -= 1) {
      const block = message.blocks[blockIndex]!;
      if (block.kind !== "question" || block.answered || block.questions.length === 0) continue;
      const { itemId } = block;
      if (itemId === undefined) continue;
      const resumeRunId = facts.awaiting.get(itemId);
      if (resumeRunId !== undefined) return { block, resumeRunId, itemId };
    }
  }
  return null;
}

export function questionCardSettledView({
  status,
  resumeRunId,
  answered,
  pending,
  answers,
}: {
  status: BlockStatus;
  resumeRunId?: string;
  answered?: boolean;
  pending: boolean;
  questions: readonly QuestionItem[];
  draft: QuestionDraft;
  answers?: QuestionAnswers;
}): QuestionCardSettledView {
  if (pending) return { settled: false };
  if (!questionSettled({ status, resumeRunId, answered })) return { settled: false };
  return { settled: true, answers };
}

export function canSubmitQuestionCard({
  resumeRunId,
  itemId,
  pending,
}: {
  resumeRunId?: string;
  itemId?: string;
  pending: boolean;
}): boolean {
  return !pending && canSubmitQuestion({ resumeRunId, itemId });
}

export function useQuestionCardActions({
  sessionId,
  resumeRunId,
  itemId,
  questions,
  draft,
}: {
  sessionId: string;
  resumeRunId?: string;
  itemId?: string;
  questions: readonly QuestionItem[];
  draft: QuestionDraft;
}) {
  const { submit, pending, submitted } = useQuestionAnswer(sessionId, resumeRunId, itemId);
  const complete = useMemo(() => questionDraftComplete(questions, draft), [questions, draft]);

  const submitAnswer = useCallback(
    (submittedDraft: QuestionDraft = draft) => {
      submit(questionDraftAnswers(questions, submittedDraft));
    },
    [draft, questions, submit],
  );

  return {
    pending,
    complete,
    disabled: submitted || !canSubmitQuestionCard({ resumeRunId, itemId, pending }),
    submit: submitAnswer,
  };
}

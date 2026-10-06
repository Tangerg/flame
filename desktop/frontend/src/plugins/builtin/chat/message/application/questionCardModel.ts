import { useCallback, useMemo } from "react";
import type { BlockStatus, ContentBlock, QuestionItem } from "@/plugins/sdk/types/contentBlock";
import type { TranscriptRow } from "@/plugins/builtin/agent/public/conversation";
import { useQuestionAnswer } from "@/plugins/builtin/agent/public/hitl";
import {
  canSubmitQuestion,
  questionDraftAnswers,
  questionDraftComplete,
  questionSettled,
  type QuestionAnswers,
  type QuestionDraft,
} from "@/plugins/builtin/agent/public/messagePresentation";

export interface QuestionCardSettledView {
  settled: boolean;
  answers?: QuestionAnswers;
}

type QuestionBlock = Extract<ContentBlock, { kind: "question" }>;

export interface PendingQuestionRequest {
  block: QuestionBlock;
  resumeRunId: string;
}

export function pendingQuestionRequest(
  rows: readonly TranscriptRow[],
): PendingQuestionRequest | null {
  for (let rowIndex = rows.length - 1; rowIndex >= 0; rowIndex -= 1) {
    const { message, facts } = rows[rowIndex]!;
    for (let blockIndex = message.blocks.length - 1; blockIndex >= 0; blockIndex -= 1) {
      const block = message.blocks[blockIndex]!;
      if (block.kind !== "question" || block.answered || block.questions.length === 0) continue;
      const resumeRunId = block.itemId === undefined ? undefined : facts.awaiting.get(block.itemId);
      if (resumeRunId !== undefined) return { block, resumeRunId };
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
  resumeRunId,
  itemId,
  questions,
  draft,
}: {
  resumeRunId?: string;
  itemId?: string;
  questions: readonly QuestionItem[];
  draft: QuestionDraft;
}) {
  const { submit, pending } = useQuestionAnswer(resumeRunId, itemId);
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
    disabled: !canSubmitQuestionCard({ resumeRunId, itemId, pending }),
    submit: submitAnswer,
  };
}

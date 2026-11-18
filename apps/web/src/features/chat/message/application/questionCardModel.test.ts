import { describe, expect, it } from "vitest";
import { type QuestionItem } from "@/features/agent/public/contentBlock";
import { createQuestionDraft, setQuestionText } from "@/features/agent/public/messagePresentation";
import { questionCardSettledView } from "@/features/chat/message/application/questionCardModel";
const question: QuestionItem = {
  type: "text",
  header: "Goal",
  prompt: "What should change?",
};

describe("questionCardSettledView", () => {
  it("uses stamped answers for completed questions", () => {
    const draft = createQuestionDraft([question]);

    expect(
      questionCardSettledView({
        status: "complete",
        answered: true,
        pending: false,
        questions: [question],
        draft,
        answers: [["Refactor"]],
      }),
    ).toEqual({ settled: true, answers: [["Refactor"]] });
  });

  it("keeps the request surface until the Runtime settles a pending submit", () => {
    const draft = setQuestionText(createQuestionDraft([question]), 0, question, "Extract model");

    expect(
      questionCardSettledView({
        status: "complete",
        resumeRunId: "run",
        pending: true,
        questions: [question],
        draft,
      }),
    ).toEqual({ settled: false });
  });

  it("never promotes a losing local draft after the Runtime settles another answer", () => {
    const draft = setQuestionText(createQuestionDraft([question]), 0, question, "local loser");

    expect(
      questionCardSettledView({
        status: "complete",
        answered: true,
        pending: false,
        questions: [question],
        draft,
        answers: [["authoritative winner"]],
      }),
    ).toEqual({ settled: true, answers: [["authoritative winner"]] });
  });

  it("shows no answer when a question closes without an accepted response", () => {
    const draft = setQuestionText(createQuestionDraft([question]), 0, question, "never accepted");

    expect(
      questionCardSettledView({
        status: "complete",
        answered: false,
        pending: false,
        questions: [question],
        draft,
      }),
    ).toEqual({ settled: true, answers: undefined });
  });
});

import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { QuestionCard } from "@/features/chat/message/ui/cards/QuestionCard";
import { type QuestionItem } from "@/features/agent/public/contentBlock";
const submitQuestion = vi.hoisted(() => vi.fn());
const answerState = vi.hoisted(() => ({ pending: false, submitted: false }));
const serviceState = vi.hoisted(() => ({ available: true }));

vi.mock("@/features/agent/public/hitl", () => ({
  useQuestionAnswer: () => ({
    submit: submitQuestion,
    pending: answerState.pending,
    submitted: answerState.submitted,
  }),
}));

vi.mock("@/features/connection/public/serviceStatus", () => ({
  useRuntimeCommandsAvailable: () => serviceState.available,
}));

const gateThenContext = [
  {
    type: "choice" as const,
    header: "Gate",
    prompt: "Which gate should run next?",
    multiple: false,
    allowCustom: false,
    options: [
      { label: "Race detector", description: "" },
      { label: "Visual suite", description: "" },
    ],
  },
  {
    type: "text" as const,
    header: "Context",
    prompt: "Describe the constraints.",
  },
] satisfies [QuestionItem, QuestionItem];

function button(name: string): HTMLButtonElement {
  return screen.getByRole("button", { name }) as HTMLButtonElement;
}

describe("QuestionCard choice semantics", () => {
  beforeEach(() => {
    submitQuestion.mockClear();
    answerState.pending = false;
    answerState.submitted = false;
    serviceState.available = true;
  });

  it("exposes single-choice answers as a labeled radio group", () => {
    render(
      <QuestionCard
        sessionId="ses-1"
        status="complete"
        resumeRunId="run-1"
        itemId="question-1"
        questions={[
          {
            type: "choice",
            header: "Gate",
            prompt: "Which gate should run next?",
            multiple: false,
            allowCustom: false,
            options: [
              { label: "Race detector", description: "Exercise concurrency paths." },
              { label: "Visual suite", description: "Verify visual states." },
            ],
          },
        ]}
      />,
    );

    expect(screen.getByRole("radiogroup", { name: "Which gate should run next?" })).toBeTruthy();
    expect(screen.getByRole("radio", { name: /Race detector/ }).getAttribute("aria-checked")).toBe(
      "true",
    );
  });

  it("exposes multi-choice answers as checkboxes", () => {
    render(
      <QuestionCard
        sessionId="ses-1"
        status="complete"
        resumeRunId="run-1"
        itemId="question-2"
        questions={[
          {
            type: "choice",
            header: "Checks",
            prompt: "Which checks should run?",
            multiple: true,
            allowCustom: false,
            options: [
              { label: "Race detector", description: "" },
              { label: "Visual suite", description: "" },
            ],
          },
        ]}
      />,
    );

    expect(screen.getAllByRole("checkbox")).toHaveLength(2);
    expect(
      screen.getByRole("checkbox", { name: "Race detector" }).getAttribute("aria-checked"),
    ).toBe("false");
  });

  it("submits the full distinguishing label when long choices share a prefix", () => {
    const labels = [
      "Run all release checks for the selected workspace and include concurrency diagnostics",
      "Run all release checks for the selected workspace and include accessibility diagnostics",
    ];
    render(
      <QuestionCard
        sessionId="ses-1"
        status="complete"
        resumeRunId="run-1"
        itemId="question-long-labels"
        questions={[
          {
            type: "choice",
            header: "Checks",
            prompt: "Which release checks should run?",
            multiple: false,
            allowCustom: false,
            options: labels.map((label) => ({ label, description: "" })),
          },
        ]}
      />,
    );

    fireEvent.click(screen.getByRole("radio", { name: labels[1] }));
    expect(screen.getByRole("radio", { name: labels[1] }).getAttribute("aria-checked")).toBe(
      "true",
    );
    expect(submitQuestion).not.toHaveBeenCalled();

    fireEvent.click(button("Submit answers"));
    expect(submitQuestion).toHaveBeenCalledTimes(1);
    expect(submitQuestion).toHaveBeenCalledWith([[labels[1]]]);
  });

  it("keeps arrow and numbered selection inside the active question", async () => {
    render(
      <QuestionCard
        sessionId="ses-1"
        status="complete"
        resumeRunId="run-1"
        itemId="question-3"
        questions={gateThenContext}
      />,
    );

    const first = screen.getByText("Race detector").closest("button");
    const second = screen.getByText("Visual suite").closest("button");
    expect(first).not.toBeNull();
    expect(second).not.toBeNull();

    first!.focus();
    fireEvent.keyDown(first!, { key: "ArrowDown" });

    await waitFor(() => {
      expect(second!.getAttribute("aria-checked")).toBe("true");
      expect(document.activeElement).toBe(second);
    });
    expect(screen.getByText("1 of 2")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Which gate should run next?" })).toBeTruthy();
    expect(screen.queryByRole("textbox")).toBeNull();

    fireEvent.keyDown(second!, { key: "1" });
    await waitFor(() => {
      expect(first!.getAttribute("aria-checked")).toBe("true");
      expect(document.activeElement).toBe(first);
    });
    expect(screen.getByText("1 of 2")).toBeTruthy();
    expect(submitQuestion).not.toHaveBeenCalled();
    expect(button("Skip").disabled).toBe(false);
    expect(button("Next").disabled).toBe(false);
  });

  it.each([
    { ctrlKey: true },
    { metaKey: true },
    { altKey: true },
    { shiftKey: true },
    { isComposing: true, keyCode: 229 },
  ])("leaves a modified or composing number key to its owner: %j", (modifiers) => {
    render(
      <QuestionCard
        sessionId="ses-1"
        status="complete"
        resumeRunId="run-1"
        itemId="question-shortcut-scope"
        questions={gateThenContext}
      />,
    );
    const selected = screen.getByRole("radio", { name: "Visual suite" });
    fireEvent.click(selected);
    act(() => selected.focus());

    expect(fireEvent.keyDown(selected, { key: "1", ...modifiers })).toBe(true);
    expect(selected.getAttribute("aria-checked")).toBe("true");
    expect(document.activeElement).toBe(selected);
    expect(screen.getByText("1 of 2")).toBeTruthy();
    expect(submitQuestion).not.toHaveBeenCalled();
  });

  it("confirms a custom answer only when the focused field has content", () => {
    render(
      <QuestionCard
        sessionId="ses-1"
        status="complete"
        resumeRunId="run-1"
        itemId="question-custom-confirmation"
        questions={[{ ...gateThenContext[0]!, type: "choice", allowCustom: true }]}
      />,
    );
    const custom = screen.getByRole("textbox", { name: "Which gate should run next?" });
    expect(button("Submit answers").disabled).toBe(false);
    fireEvent.keyDown(custom, { key: "Enter", keyCode: 13 });
    expect(submitQuestion).not.toHaveBeenCalled();

    fireEvent.change(custom, { target: { value: "Only the relevant checks." } });
    fireEvent.keyDown(custom, { key: "Enter", keyCode: 13 });
    expect(submitQuestion).toHaveBeenCalledExactlyOnceWith([["Only the relevant checks."]]);
  });

  it("clears the radio selection when a skipped question is answered with custom text", async () => {
    render(
      <QuestionCard
        sessionId="ses-1"
        status="complete"
        resumeRunId="run-1"
        itemId="question-skipped-custom"
        questions={[{ ...gateThenContext[0], allowCustom: true }, gateThenContext[1]]}
      />,
    );
    fireEvent.click(button("Skip"));
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("textbox", { name: "Describe the constraints." }),
      ),
    );
    fireEvent.click(button("Previous question"));
    const first = screen.getByRole("radio", { name: "Race detector" });
    await waitFor(() => expect(document.activeElement).toBe(first));
    const second = screen.getByRole("radio", { name: "Visual suite" });
    fireEvent.click(second);
    expect(second.getAttribute("aria-checked")).toBe("true");

    fireEvent.change(screen.getByRole("textbox", { name: "Which gate should run next?" }), {
      target: { value: "Only the relevant checks." },
    });

    expect(first.getAttribute("aria-checked")).toBe("false");
    expect(second.getAttribute("aria-checked")).toBe("false");
    expect(button("Next").disabled).toBe(false);
    expect(submitQuestion).not.toHaveBeenCalled();
  });

  it("uses the question as the only request heading and keeps option detail with its choice", () => {
    const { container } = render(
      <QuestionCard
        sessionId="ses-1"
        status="complete"
        resumeRunId="run-1"
        itemId="question-4"
        questions={[
          {
            type: "choice",
            header: "Gate",
            prompt: "Which gate should run next?",
            multiple: false,
            allowCustom: false,
            options: [
              {
                label: "Race detector",
                description: "Exercise concurrency paths.",
                preview: "race-preview",
              },
              {
                label: "Visual suite",
                description: "Verify visual states.",
                preview: "visual-preview",
              },
            ],
          },
        ]}
      />,
    );

    const surface = container.querySelector<HTMLElement>('[data-slot="question-request-surface"]');
    expect(surface).not.toBeNull();
    expect(
      within(surface!).getByRole("heading", { name: "Which gate should run next?" }),
    ).toBeTruthy();
    expect(within(surface!).getByRole("radio", { name: /Race detector/ }).textContent).toContain(
      "Exercise concurrency paths.",
    );
    expect(screen.queryByText("Input needed")).toBeNull();
    expect(screen.queryByText("Gate")).toBeNull();
    expect(screen.queryByRole("region", { name: "Race detector" })).toBeNull();
    expect(screen.queryByText("race-preview")).toBeNull();
    expect(screen.queryByText("visual-preview")).toBeNull();
  });

  it("gives a freeform question a multiline answer surface", () => {
    render(
      <QuestionCard
        sessionId="ses-1"
        status="complete"
        resumeRunId="run-1"
        itemId="question-5"
        questions={[
          {
            type: "text",
            header: "Context",
            prompt: "Describe the constraints for this change.",
          },
        ]}
      />,
    );

    const answer = screen.getByRole("textbox", {
      name: "Describe the constraints for this change.",
    });
    expect(answer.tagName).toBe("TEXTAREA");
    expect(answer.getAttribute("rows")).toBe("4");
  });

  it("folds settled answers into Codex's compact question disclosure", () => {
    const { container } = render(
      <QuestionCard
        sessionId="ses-1"
        status="complete"
        itemId="question-6"
        answered
        answers={[["First constraint\nSecond constraint"]]}
        questions={[
          {
            type: "text",
            header: "Context",
            prompt: "Describe the constraints for this change.",
          },
        ]}
      />,
    );

    const disclosure = container.querySelector<HTMLElement>(
      '[data-slot="agent-activity-disclosure"]',
    );
    expect(disclosure).not.toBeNull();
    expect(disclosure!.getAttribute("data-shell")).toBe("line");

    const trigger = screen.getByRole("button", { name: "Asked 1 question" });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByText("Describe the constraints for this change.")).toBeNull();
    expect(screen.queryByText("First constraint\nSecond constraint")).toBeNull();

    fireEvent.click(trigger);

    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByText("Describe the constraints for this change.")).toBeTruthy();
    const answer = screen.getByText(/First constraint\s+Second constraint/);
    expect(answer.dataset.settledAnswer).toBe("");
  });

  it("confirms each question explicitly and keeps drafts while paging", async () => {
    render(
      <QuestionCard
        sessionId="ses-1"
        status="complete"
        resumeRunId="run-1"
        itemId="question-7"
        questions={[
          {
            type: "choice",
            header: "Checks",
            prompt: "Which checks should run?",
            multiple: true,
            allowCustom: false,
            options: [
              { label: "Race detector", description: "" },
              { label: "Visual suite", description: "" },
            ],
          },
          {
            type: "text",
            header: "Context",
            prompt: "Describe the constraints.",
          },
        ]}
      />,
    );

    expect(screen.getByText("1 of 2")).toBeTruthy();
    expect(button("Next").disabled).toBe(true);
    expect(button("Skip").disabled).toBe(false);

    fireEvent.click(screen.getByRole("checkbox", { name: "Race detector" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Visual suite" }));
    expect(screen.getByText("1 of 2")).toBeTruthy();
    expect(screen.queryByText("Describe the constraints.")).toBeNull();
    expect(submitQuestion).not.toHaveBeenCalled();

    fireEvent.click(button("Next"));
    expect(screen.getByText("2 of 2")).toBeTruthy();
    const answer = screen.getByRole("textbox", { name: "Describe the constraints." });
    await waitFor(() => expect(document.activeElement).toBe(answer));
    expect(button("Submit answers").disabled).toBe(true);
    expect(button("Skip").disabled).toBe(false);

    fireEvent.keyDown(answer, { key: "Enter", keyCode: 13 });
    expect(screen.getByText("2 of 2")).toBeTruthy();
    expect(submitQuestion).not.toHaveBeenCalled();

    fireEvent.change(answer, { target: { value: "Keep the boundary exact." } });
    fireEvent.click(button("Previous question"));
    expect(screen.getByText("1 of 2")).toBeTruthy();
    expect(
      screen.getByRole("checkbox", { name: "Race detector" }).getAttribute("aria-checked"),
    ).toBe("true");
    expect(
      screen.getByRole("checkbox", { name: "Visual suite" }).getAttribute("aria-checked"),
    ).toBe("true");

    fireEvent.click(button("Next question"));
    expect(
      (screen.getByRole("textbox", { name: "Describe the constraints." }) as HTMLTextAreaElement)
        .value,
    ).toBe("Keep the boundary exact.");

    fireEvent.click(button("Submit answers"));
    expect(submitQuestion).toHaveBeenCalledTimes(1);
    expect(submitQuestion).toHaveBeenCalledWith([
      ["Race detector", "Visual suite"],
      ["Keep the boundary exact."],
    ]);
  });

  it("skips only the active answer", () => {
    render(
      <QuestionCard
        sessionId="ses-1"
        status="complete"
        resumeRunId="run-1"
        itemId="question-skip"
        questions={gateThenContext}
      />,
    );

    fireEvent.click(screen.getByRole("radio", { name: "Visual suite" }));
    fireEvent.click(button("Skip"));
    expect(screen.getByText("2 of 2")).toBeTruthy();
    expect(submitQuestion).not.toHaveBeenCalled();

    fireEvent.change(screen.getByRole("textbox", { name: "Describe the constraints." }), {
      target: { value: "Keep the boundary exact." },
    });
    fireEvent.click(button("Submit answers"));
    expect(submitQuestion).toHaveBeenCalledTimes(1);
    expect(submitQuestion).toHaveBeenCalledWith([[], ["Keep the boundary exact."]]);
  });

  it("returns to the selected radio while keeping the other question's draft", async () => {
    render(
      <QuestionCard
        sessionId="ses-1"
        status="complete"
        resumeRunId="run-1"
        itemId="question-return-selection"
        questions={gateThenContext}
      />,
    );
    fireEvent.click(screen.getByRole("radio", { name: "Visual suite" }));
    fireEvent.click(button("Next"));
    fireEvent.change(screen.getByRole("textbox", { name: "Describe the constraints." }), {
      target: { value: "Keep the boundary exact." },
    });

    fireEvent.click(button("Previous question"));
    const selected = screen.getByRole("radio", { name: "Visual suite" });
    expect(selected.getAttribute("aria-checked")).toBe("true");
    await waitFor(() => {
      expect(document.activeElement).toBe(selected);
      expect(selected.tabIndex).toBe(0);
    });
    expect(submitQuestion).not.toHaveBeenCalled();

    fireEvent.click(button("Next question"));
    expect(
      (screen.getByRole("textbox", { name: "Describe the constraints." }) as HTMLTextAreaElement)
        .value,
    ).toBe("Keep the boundary exact.");
  });

  it("leaves later outside focus alone while the previous question registers", async () => {
    render(
      <>
        <button>Outside question</button>
        <QuestionCard
          sessionId="ses-1"
          status="complete"
          resumeRunId="run-1"
          itemId="question-later-focus"
          questions={gateThenContext}
        />
      </>,
    );
    fireEvent.click(button("Next"));
    await waitFor(() => expect(document.activeElement?.tagName).toBe("TEXTAREA"));
    const outside = button("Outside question");

    await act(async () => {
      fireEvent.click(button("Previous question"));
      outside.focus();
    });

    expect(screen.getByText("1 of 2")).toBeTruthy();
    expect(document.activeElement).toBe(outside);
    expect(submitQuestion).not.toHaveBeenCalled();
  });

  it("disables skip and confirmation when the answer cannot be sent", () => {
    serviceState.available = false;
    const { unmount } = render(
      <QuestionCard
        sessionId="ses-1"
        status="complete"
        resumeRunId="run-1"
        itemId="question-offline"
        questions={gateThenContext}
      />,
    );

    expect(button("Skip").disabled).toBe(true);
    expect(button("Next").disabled).toBe(true);
    unmount();

    serviceState.available = true;
    answerState.submitted = true;
    render(
      <QuestionCard
        sessionId="ses-1"
        status="complete"
        resumeRunId="run-1"
        itemId="question-submitted"
        questions={gateThenContext.slice(1)}
      />,
    );

    fireEvent.change(screen.getByRole("textbox", { name: "Describe the constraints." }), {
      target: { value: "Keep the boundary exact." },
    });
    expect(button("Skip").disabled).toBe(true);
    expect(button("Submit answers").disabled).toBe(true);
    fireEvent.keyDown(screen.getByRole("textbox", { name: "Describe the constraints." }), {
      key: "Enter",
      keyCode: 13,
    });
    expect(submitQuestion).not.toHaveBeenCalled();
  });

  it("keeps pending answers immutable and both completion actions disabled", () => {
    answerState.pending = true;
    render(
      <QuestionCard
        sessionId="ses-1"
        status="complete"
        resumeRunId="run-1"
        itemId="question-pending"
        questions={gateThenContext}
      />,
    );
    const selected = screen.getByRole("radio", { name: "Race detector" });
    fireEvent.click(screen.getByRole("radio", { name: "Visual suite" }));
    fireEvent.keyDown(selected, { key: "2" });
    expect(selected.getAttribute("aria-checked")).toBe("true");
    expect(button("Skip").disabled).toBe(true);
    expect(button("Next").disabled).toBe(true);
    fireEvent.click(button("Skip"));
    fireEvent.click(button("Next"));
    expect(screen.getByText("1 of 2")).toBeTruthy();
    expect(submitQuestion).not.toHaveBeenCalled();
  });

  it("keeps an unmarked Chinese-IME commit Enter inside the freeform answer", () => {
    render(
      <QuestionCard
        sessionId="ses-1"
        status="complete"
        resumeRunId="run-1"
        itemId="question-ime"
        questions={[
          {
            type: "text",
            header: "Context",
            prompt: "Describe the constraint.",
          },
        ]}
      />,
    );

    const answer = screen.getByRole("textbox", { name: "Describe the constraint." });
    fireEvent.compositionStart(answer, { data: "english" });
    fireEvent.change(answer, { target: { value: "中文 english" } });
    fireEvent.compositionEnd(answer, { data: "english" });
    fireEvent.keyDown(answer, { key: "Enter", keyCode: 13, isComposing: false });

    expect(submitQuestion).not.toHaveBeenCalled();

    fireEvent.keyDown(answer, { key: "Enter", keyCode: 13, isComposing: false });
    expect(submitQuestion).toHaveBeenCalledWith([["中文 english"]]);
  });
});

import { type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type TranscriptRow } from "@/features/agent/public/conversation";
import { selectAgentSession } from "@/features/agent/public/session";
import {
  beginComposerDraftReplacement,
  captureComposerSubmission,
  replaceComposerDraft,
} from "@/features/composer/public/draft";
import { focusComposer } from "@/features/composer/public/focus";
import { ChatStream } from "@/features/shell/workbench/panel/ChatStream";
import { ContentVisibility } from "@/ui/index";

const material = vi.hoisted(() => ({ rows: [] as TranscriptRow[] }));

vi.mock("@/features/agent/public/conversation", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agent/public/conversation")>()),
  useActiveConversationRows: () => material.rows,
}));

vi.mock("@/features/connection/public/serviceStatus", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/connection/public/serviceStatus")>()),
  useRuntimeServiceStatus: () => ({ phase: "ready", observation: null, failure: null }),
  useRuntimeCommandsAvailable: () => true,
  runtimeCommandsAvailable: () => true,
}));

vi.mock("@/features/agent/public/hitl", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agent/public/hitl")>()),
  useQuestionAnswer: () => ({ submit: () => undefined, pending: false, submitted: false }),
}));

function userRow(): TranscriptRow {
  return {
    message: {
      id: "user-1",
      runId: "run-1",
      role: "user",
      blocks: [{ kind: "text", status: "complete", text: "Earlier request" }],
    },
    runOwner: { kind: "owned", runId: "run-1", status: "finished" },
    facts: { toolCalls: {}, delegatedRuns: {}, awaiting: new Map() },
  };
}

function questionRow(itemId: string): TranscriptRow {
  return {
    message: {
      id: "question-message",
      runId: "run-1",
      role: "assistant",
      blocks: [
        {
          kind: "question",
          itemId,
          status: "complete",
          questions: [
            {
              type: "choice",
              header: "Checks",
              prompt: "Which checks should run?",
              multiple: true,
              allowCustom: false,
              options: [{ label: "Visual suite", description: "Verify visual states." }],
            },
          ],
        },
      ],
    },
    runOwner: { kind: "owned", runId: "run-1", status: "waiting" },
    facts: { toolCalls: {}, delegatedRuns: {}, awaiting: new Map([[itemId, "run-1"]]) },
  };
}

function Harness({ visible = true }: { visible?: boolean }) {
  return (
    <>
      <button>Later control</button>
      <ContentVisibility visible={visible}>
        <ChatStream onSend={() => true} />
      </ContentVisibility>
    </>
  );
}

function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity } },
  });
  const view = render(<Harness />, {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    ),
  });
  return {
    ...view,
    showRows(rows: TranscriptRow[], visible = true) {
      material.rows = rows;
      view.rerender(<Harness visible={visible} />);
    },
  };
}

beforeEach(() => {
  material.rows = [userRow()];
  selectAgentSession("stream-session");
  replaceComposerDraft({ text: "Keep this authored draft" });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("ChatStream composer lifetime", () => {
  it("keeps the same input, selection, and focus across both conversation layouts", () => {
    const view = mount();
    const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message composer" });
    fireEvent.change(draft, { target: { value: "Still editing this draft" } });
    act(() => {
      draft.focus();
      draft.setSelectionRange(6, 13, "backward");
    });

    view.showRows([]);

    expect(screen.getByRole("textbox", { name: "Message composer" })).toBe(draft);
    expect(document.activeElement).toBe(draft);
    expect(draft.value).toBe("Still editing this draft");
    expect([draft.selectionStart, draft.selectionEnd, draft.selectionDirection]).toEqual([
      6,
      13,
      "backward",
    ]);
    expect(screen.getByRole("heading", { name: "What should we build?" })).toBeTruthy();

    view.showRows([userRow()]);

    expect(screen.getByRole("textbox", { name: "Message composer" })).toBe(draft);
    expect(document.activeElement).toBe(draft);
    expect([draft.selectionStart, draft.selectionEnd, draft.selectionDirection]).toEqual([
      6,
      13,
      "backward",
    ]);
    act(() => screen.getByRole("button", { name: "Later control" }).focus());
    act(() => focusComposer());
    expect(document.activeElement).toBe(draft);
    expect(captureComposerSubmission().value).toBe("Still editing this draft");
  });

  it("keeps the registered input focused when history return fills the empty conversation", () => {
    const view = mount();
    const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message composer" });
    let replace!: ReturnType<typeof beginComposerDraftReplacement>;
    act(() => {
      replace = beginComposerDraftReplacement();
    });

    view.showRows([]);
    let replaced = false;
    act(() => {
      replaced = replace({ text: "Restored history input" });
    });

    expect(replaced).toBe(true);
    expect(screen.getByRole("textbox", { name: "Message composer" })).toBe(draft);
    expect(draft.value).toBe("Restored history input");
    expect(document.activeElement).toBe(draft);
    expect([draft.selectionStart, draft.selectionEnd]).toEqual([
      draft.value.length,
      draft.value.length,
    ]);
  });

  it("keeps later outside focus when a successful replacement empties the conversation", () => {
    const view = mount();
    const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message composer" });
    let replace!: ReturnType<typeof beginComposerDraftReplacement>;
    act(() => {
      replace = beginComposerDraftReplacement();
    });
    const outside = screen.getByRole("button", { name: "Later control" });
    act(() => outside.focus());

    view.showRows([]);
    let replaced = false;
    act(() => {
      replaced = replace({ text: "Restored history input" });
    });

    expect(replaced).toBe(true);
    expect(screen.getByRole("textbox", { name: "Message composer" })).toBe(draft);
    expect(draft.value).toBe("Restored history input");
    expect(document.activeElement).toBe(outside);
  });

  it("retains a question's selected answer until its request identity changes", () => {
    material.rows = [questionRow("question-1")];
    const view = mount();
    const answer = screen.getByRole("checkbox", { name: "Visual suite" });
    fireEvent.click(answer);
    expect(answer.getAttribute("aria-checked")).toBe("true");

    view.showRows([userRow(), questionRow("question-1")]);

    expect(screen.getByRole("checkbox", { name: "Visual suite" })).toBe(answer);
    expect(answer.getAttribute("aria-checked")).toBe("true");

    view.showRows([userRow(), questionRow("question-2")]);

    expect(screen.getByRole("checkbox", { name: "Visual suite" })).not.toBe(answer);
    expect(
      screen.getByRole("checkbox", { name: "Visual suite" }).getAttribute("aria-checked"),
    ).toBe("false");
  });
});

describe("ChatStream question focus handoff", () => {
  function focusQuestion() {
    const answer = screen.getByRole("checkbox", { name: "Visual suite" });
    act(() => answer.focus());
    expect(document.activeElement).toBe(answer);
  }

  beforeEach(() => {
    material.rows = [userRow(), questionRow("question-1")];
  });

  it("moves focus from a retired question to the Composer that replaces it", () => {
    const view = mount();
    focusQuestion();

    view.showRows([userRow()]);

    const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message composer" });
    expect(document.activeElement).toBe(draft);
    expect(draft.value).toBe("Keep this authored draft");

    act(() => draft.blur());
    view.showRows([userRow()]);

    expect(document.activeElement).toBe(document.body);
  });

  it("keeps later outside focus when the question retires", () => {
    const view = mount();
    focusQuestion();
    const outside = screen.getByRole("button", { name: "Later control" });
    act(() => outside.focus());

    view.showRows([userRow()]);

    expect(document.activeElement).toBe(outside);
  });

  it("does not hand the old question's focus to another Session's Composer", () => {
    mount();
    focusQuestion();

    act(() => {
      material.rows = [userRow()];
      selectAgentSession("other-session");
    });

    expect(screen.getByRole("textbox", { name: "Message composer" })).toBeTruthy();
    expect(document.activeElement).toBe(document.body);
  });

  it("does not move focus into hidden content or replay it on reveal", () => {
    const view = mount();
    focusQuestion();

    view.showRows([userRow()], false);
    expect(document.activeElement).toBe(document.body);

    view.showRows([userRow()]);
    expect(document.activeElement).toBe(document.body);
  });

  it("retires hidden question focus even while the question stays pending", () => {
    const view = mount();
    focusQuestion();
    const answer = screen.getByRole("checkbox", { name: "Visual suite" });

    view.showRows([userRow(), questionRow("question-1")], false);
    act(() => answer.blur());
    view.showRows([userRow(), questionRow("question-1")]);
    view.showRows([userRow()]);

    expect(document.activeElement).toBe(document.body);
  });
});

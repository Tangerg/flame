import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, screen } from "@testing-library/react";
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
import { AgentAppShell } from "@/ui/agent/index";
import { COMPOSER_OVERLAY_PROPERTY } from "@/features/shell/workbench/panel/composerOverlay";

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
    <AgentAppShell
      sidebarLabel="Navigation"
      sidebarResizeLabel="Resize navigation"
      sidebarOpen={false}
      sidebarWidth={275}
      onResize={() => undefined}
      onSidebarToggle={() => undefined}
      sidebarExpandLabel="Expand navigation"
      sidebarCollapseLabel="Collapse navigation"
      main={
        <>
          <button>Later control</button>
          <ContentVisibility visible={visible}>
            <ChatStream onSend={() => true} />
          </ContentVisibility>
        </>
      }
    />
  );
}

function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity } },
  });
  const view = testClient.render(<Harness />, {
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
  selectAgentSession(testClient.agent.access, "stream-session");
  replaceComposerDraft(testClient.composer.access, { text: "Keep this authored draft" });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function measureStream() {
  const observers: TestResizeObserver[] = [];
  class TestResizeObserver {
    readonly nodes = new Set<Element>();
    active = true;
    constructor(private readonly callback: ResizeObserverCallback) {
      observers.push(this);
    }
    observe(node: Element) {
      this.nodes.add(node);
    }
    unobserve(node: Element) {
      this.nodes.delete(node);
    }
    disconnect() {
      this.active = false;
    }
    notify(entries: ResizeObserverEntry[] = []) {
      act(() => this.callback(entries, this as unknown as ResizeObserver));
    }
  }
  vi.stubGlobal("ResizeObserver", TestResizeObserver);
  vi.spyOn(document.documentElement, "clientHeight", "get").mockReturnValue(600);
  return (input: Element) => {
    const observer = observers.find(
      (entry) => entry.active && [...entry.nodes].filter((node) => node.contains(input)).length > 1,
    )!;
    expect(observer).toBeTruthy();
    const ancestors = [...observer.nodes].filter((node) => node.contains(input));
    const layout = ancestors.find((node) => ancestors.every((other) => node.contains(other)))!;
    const overlay = ancestors.find(
      (node) => !ancestors.some((other) => other !== node && node.contains(other)),
    )!;
    const jump = [...observer.nodes].find(
      (node) => !node.contains(input) && node.querySelector('[aria-label="Jump to bottom"]'),
    );
    return { observer, layout, overlay, jump };
  };
}

function measuredRect(node: Element, top: number, bottom: number) {
  return vi
    .spyOn(node, "getBoundingClientRect")
    .mockReturnValue(new DOMRect(0, top, 600, bottom - top));
}

describe("ChatStream notification space", () => {
  it("publishes space outside the composer and jump control, then retires hidden measurements", () => {
    const measured = measureStream();
    const view = mount();
    const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message composer" });
    const shell = view.container.querySelector<HTMLElement>(".agent-shell")!;
    expect(shell.style.getPropertyValue("--notification-top")).not.toBe("");
    const { observer, layout, overlay, jump } = measured(draft);
    expect(jump).toBeTruthy();
    measuredRect(layout, 46, 600);
    const overlayRect = measuredRect(overlay, 460, 600);
    const jumpRect = measuredRect(jump!, 420, 452);
    act(() => {
      draft.focus();
      draft.setSelectionRange(5, 10);
    });
    observer.notify();

    expect(shell.style.getPropertyValue("--notification-top")).toBe("46px");
    expect(shell.style.getPropertyValue("--notification-bottom")).toBe("180px");
    const pane = view.container.querySelector<HTMLElement>('[style*="--composer-overlay:"]')!;
    expect(pane.style.getPropertyValue(COMPOSER_OVERLAY_PROPERTY)).toBe("140px");

    overlayRect.mockReturnValue(new DOMRect(0, 300, 600, 300));
    jumpRect.mockReturnValue(new DOMRect(0, 260, 600, 32));
    observer.notify();
    expect(shell.style.getPropertyValue("--notification-bottom")).toBe("340px");
    expect(pane.style.getPropertyValue(COMPOSER_OVERLAY_PROPERTY)).toBe("300px");
    expect(document.activeElement).toBe(draft);
    expect([draft.selectionStart, draft.selectionEnd]).toEqual([5, 10]);

    view.showRows([userRow()], false);
    expect(shell.style.getPropertyValue("--notification-top")).toBe("");
    expect(shell.style.getPropertyValue("--notification-bottom")).toBe("");
    expect(pane.style.getPropertyValue(COMPOSER_OVERLAY_PROPERTY)).toBe("");

    view.showRows([userRow()]);
    expect(screen.getByRole("textbox", { name: "Message composer" })).toBe(draft);
    expect(draft.value).toBe("Keep this authored draft");
    expect(shell.style.getPropertyValue("--notification-bottom")).toBe("340px");
    view.unmount();
    expect(shell.style.getPropertyValue("--notification-top")).toBe("");
    expect(shell.style.getPropertyValue("--notification-bottom")).toBe("");
  });

  it("follows a centered composer's scrolling position and sibling layout changes", () => {
    material.rows = [];
    const measured = measureStream();
    const view = mount();
    const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message composer" });
    const shell = view.container.querySelector<HTMLElement>(".agent-shell")!;
    const { observer, layout, overlay } = measured(draft);
    measuredRect(layout, 46, 600);
    const overlayRect = measuredRect(overlay, 200, 300);
    observer.notify();
    expect(shell.style.getPropertyValue("--notification-top")).toBe("300px");
    expect(shell.style.getPropertyValue("--notification-bottom")).toBe("0px");

    overlayRect.mockReturnValue(new DOMRect(0, 420, 600, 100));
    const scroller = view.container.querySelector('[data-slot="empty-conversation-layout"]')!;
    expect(scroller).not.toBe(layout);
    fireEvent.scroll(scroller, { bubbles: false });
    expect(shell.style.getPropertyValue("--notification-top")).toBe("46px");
    expect(shell.style.getPropertyValue("--notification-bottom")).toBe("180px");

    overlayRect.mockReturnValue(new DOMRect(0, 160, 600, 100));
    const heading = [...observer.nodes].find((node) => node.querySelector("h2"))!;
    observer.notify([{ target: heading } as ResizeObserverEntry]);
    expect(shell.style.getPropertyValue("--notification-top")).toBe("260px");
    expect(shell.style.getPropertyValue("--notification-bottom")).toBe("0px");
    expect(screen.getByRole("textbox", { name: "Message composer" })).toBe(draft);
    expect(draft.value).toBe("Keep this authored draft");
  });
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
    act(() => focusComposer(testClient.composer.access));
    expect(document.activeElement).toBe(draft);
    expect(captureComposerSubmission(testClient.composer.access).value).toBe(
      "Still editing this draft",
    );
  });

  it("keeps the registered input focused when history return fills the empty conversation", () => {
    const view = mount();
    const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message composer" });
    let replace!: ReturnType<typeof beginComposerDraftReplacement>;
    act(() => {
      replace = beginComposerDraftReplacement(testClient.composer.access);
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
      replace = beginComposerDraftReplacement(testClient.composer.access);
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
      selectAgentSession(testClient.agent.access, "other-session");
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

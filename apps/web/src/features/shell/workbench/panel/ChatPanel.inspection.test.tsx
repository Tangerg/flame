import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { mockSelectLayout } from "@/test/selectLayout";
let testClient: TestClient;
prepareTestClient(() => {
  mockSelectLayout();
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen, within } from "@testing-library/react";
import { createRef, useEffect, useRef, useState } from "react";
import { ActionMenu, ModalDialog, SearchField, Select } from "@flame/ui";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type WorkspaceViewSpec } from "@/features/workspace/public/views";
import { type Message } from "@/features/agent/public/sessionView";

import { FileRefLink } from "@/features/workspace/public/FileRefLink";
import { MessageTranscript } from "@/features/chat/public/message/rendering";
import {
  activateWorkspaceSessionScope,
  openWorkspaceFile,
  openWorkspaceSubagentRun,
  openWorkspaceView,
  openWorkspaceViewInDock,
  showWorkspaceDock,
  toggleWorkspaceDock,
  useWorkspaceFileViewer,
  useWorkspaceSubagentRunId,
} from "@/features/workspace/public/navigation";
import { t } from "@/lib/i18n/index";
import { useDockWidth } from "@/features/workspace/public/sidebarDrawer";
import { ChatPanel, type ChatPanelHandle } from "@/features/shell/workbench/panel/ChatPanel";
import { registerComposerFocusTarget } from "@/features/composer/public/focus";
import {
  appendToComposerDraft,
  beginComposerDraftReplacement,
  captureComposerSubmission,
  replaceComposerDraft,
  useComposerText,
  useSetComposerText,
} from "@/features/composer/public/draft";
import { writeToClipboard } from "@/lib/clipboard";
import { drainBrowserTasks } from "@/test/browserTasks";
import { ContextMenu, Popover } from "@/ui/index";
const TABLE = "| File | Note |\n|---|---|\n| app/ui/resizer.ts:6 | README.md:4 |";
const INITIAL_DRAFT = "Keep this draft — 保留";
const CHILD_TEXT =
  "Inspect the complete module graph and explain where persistence belongs. Trace the public entry points, their callers, and the existing regression coverage before proposing a change. Preserve the full task instructions when returning to the composer, including this final sentence about ownership and failure behavior.";
const rootPrompt: Message = {
  id: "root-prompt",
  runId: "root-run",
  role: "user",
  blocks: [{ kind: "text", text: "Revise the root task input.", status: "complete" }],
};
const childPrompt: Message = {
  id: "child-prompt",
  runId: "child-run",
  role: "user",
  blocks: [{ kind: "text", text: CHILD_TEXT, status: "complete" }],
};

function DraftInput() {
  const input = useRef<HTMLTextAreaElement>(null);
  const value = useComposerText();
  const setValue = useSetComposerText();
  useEffect(() => registerComposerFocusTarget(testClient.composer.access, input.current), []);
  return (
    <textarea
      ref={input}
      aria-label="Composer draft"
      value={value}
      onChange={(event) => setValue(event.target.value)}
    />
  );
}

function PromptTurn({ message }: { message: Message }) {
  return (
    <MessageTranscript
      rows={[
        {
          message,
          runOwner: { kind: "owned", runId: message.runId!, status: "finished" },
          facts: { toolCalls: {}, delegatedRuns: {}, awaiting: new Map<string, string>() },
        },
      ]}
      ctx={{
        sessionId: "ses_first",
        expandedIds: new Set(),
        onToggleExpand: () => {},
        textReveal: "instant",
      }}
      sessionId="ses_first"
      isRunning={false}
    />
  );
}

function AnswerTurn({ text }: { text: string }) {
  return (
    <MessageTranscript
      rows={[
        {
          message: {
            id: "answer",
            runId: "run-answer",
            role: "assistant",
            blocks: [{ kind: "text", itemId: "answer-text", text, status: "complete" }],
          },
          runOwner: { kind: "owned", runId: "run-answer", status: "finished" },
          facts: { toolCalls: {}, delegatedRuns: {}, awaiting: new Map<string, string>() },
        },
      ]}
      ctx={{
        sessionId: "ses_first",
        expandedIds: new Set(),
        onToggleExpand: () => {},
        textReveal: "instant",
      }}
      sessionId="ses_first"
      isRunning={false}
    />
  );
}

const model = vi.hoisted(() => ({
  sessionId: "ses_first",
  views: [] as WorkspaceViewSpec[],
  messages: [] as Message[],
  rollback: vi.fn(),
}));

vi.mock("@/features/agent/public/session", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agent/public/session")>()),
  useActiveSession: () => null,
  useActiveSessionId: () => model.sessionId,
  useAgentSessions: () => ({ isLoading: false, data: [] }),
}));
prepareTestClient(() => {
  Object.assign(testClient.agent.access, {
    activeAgentConversation: () => ({ sessionId: model.sessionId, messages: model.messages }),
    rollbackSessionToBeforeRun: model.rollback,
  });
});

vi.mock("@/features/agent/public/conversation", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agent/public/conversation")>()),
  useActiveConversationMessages: () => model.messages,
}));

vi.mock("@/lib/clipboard", () => ({ writeToClipboard: vi.fn().mockResolvedValue(true) }));

vi.mock("@/features/workspace/public/contextDockCatalog", () => ({
  useContextDockCatalog: () => [
    {
      id: "workspace",
      title: "Workspace",
      destinations: model.views.map((view) => ({
        viewId: view.id,
        title: view.title,
        scope: "workspace",
      })),
    },
  ],
}));

vi.mock("@/features/workspace/public/views", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/features/workspace/public/views")>();
  return { ...actual, useWorkspaceViews: () => model.views };
});

vi.mock("@/extensions/host/Slot", () => ({ Slot: () => null }));

vi.mock("@/features/shell/workbench/panel/HeaderDiffStat", () => ({ HeaderDiffStat: () => null }));

vi.mock("@/features/shell/workbench/panel/RunStatusPill", () => ({ RunStatusPill: () => null }));

vi.mock("@/features/shell/workbench/panel/RunAnnouncer", () => ({ RunAnnouncer: () => null }));

vi.mock("@/features/shell/workbench/panel/ChatStream", async () => {
  const { FileRefLink } = await import("@/features/workspace/public/FileRefLink");
  return {
    ChatStream: () => (
      <>
        <FileRefLink path="src/main.ts" line={4} />
        <PromptTurn message={rootPrompt} />
        <AnswerTurn text={"| Path |\n|---|\n| src/stream.ts:2 |\n| docs/selected.md:9 |"} />
        <button
          onClick={(event) =>
            openWorkspaceSubagentRun(testClient.workspace.access, "child", event.currentTarget)
          }
        >
          Inspect task
        </button>
        <button>Other control</button>
        <DraftInput />
      </>
    ),
  };
});

function ChildReader() {
  const [count, setCount] = useState(0);
  const runId = useWorkspaceSubagentRunId();
  return (
    <>
      <button onClick={() => setCount(count + 1)}>Evidence {count}</button>
      <span>Selected task: {runId}</span>
      <button key={runId}>Read {runId}</button>
      <PromptTurn message={childPrompt} />
      <button
        onClick={(event) =>
          openWorkspaceSubagentRun(testClient.workspace.access, "nested", event.currentTarget)
        }
      >
        Inspect nested task
      </button>
      <FileRefLink path="src/child.ts" line={8} />
      <AnswerTurn text={TABLE} />
    </>
  );
}

function FilteredMaterial() {
  const [query, setQuery] = useState("src/main");
  return (
    <SearchField
      value={query}
      onValueChange={setQuery}
      onClear={() => setQuery("")}
      clearLabel="Clear filter"
      aria-label="Filter material"
    />
  );
}

async function openPreview(scope: HTMLElement) {
  const expand = await within(scope).findByRole("button", { name: "Expand table" });
  act(() => expand.focus());
  fireEvent.click(expand);
  return { expand, dialog: screen.getByRole("dialog", { name: "Table preview" }) };
}

async function settle() {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

function windowHeader() {
  return document.querySelector<HTMLElement>('[data-slot="workspace-window-header"]');
}

function observeRowWidth(initial: number) {
  let width = initial;
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockImplementation(() => width);
  const observers: ResizeObserverCallback[] = [];
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(callback: ResizeObserverCallback) {
        observers.push(callback);
      }
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  return async (next: number) => {
    width = next;
    await act(async () => {
      observers.forEach((observe) => observe([], {} as ResizeObserver));
      await new Promise((resolve) => requestAnimationFrame(resolve));
    });
  };
}

beforeEach(() => {
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(1200);
  model.sessionId = "ses_first";
  model.messages = [rootPrompt];
  model.rollback.mockReset().mockResolvedValue({
    status: "committed",
    userInput: { parts: [{ kind: "text", text: "Canonical root input" }] },
  });
  vi.mocked(writeToClipboard).mockClear();
  testClient.agent.access.selectAgentSession(model.sessionId);
  replaceComposerDraft(testClient.composer.access, { text: INITIAL_DRAFT });
  activateWorkspaceSessionScope(testClient.workspace.access, model.sessionId);
  model.views = [
    {
      id: "file",
      title: "Files",
      dock: "workspace",
      component: () => <button>File controls</button>,
    },
    { id: "subagents", title: "Subagents", dock: "workspace", component: ChildReader },
  ];
});

afterEach(async () => {
  cleanup();
  await drainBrowserTasks();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("hands focus from a removed full-width trigger to the material and back to the conversation", () => {
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(700);
  openWorkspaceViewInDock(testClient.workspace.access, "file");
  testClient.render(<ChatPanel onSend={() => true} />);
  const trigger = screen.getByRole("button", { name: /Open material full width/ });
  act(() => trigger.focus());
  fireEvent.click(trigger);
  expect(document.activeElement).toBe(screen.getByRole("region", { name: "Files" }));

  const close = screen.getByRole("button", { name: "Close" });
  act(() => close.focus());
  fireEvent.click(close);
  expect(document.activeElement).toBe(screen.getByRole("region", { name: "Conversation" }));
});

it("returns focus from a removed main header to the visible Dock", () => {
  openWorkspaceViewInDock(testClient.workspace.access, "file");
  openWorkspaceView(testClient.workspace.access, "file");
  testClient.render(<ChatPanel onSend={() => true} />);
  const close = screen.getByRole("button", { name: "Close" });
  act(() => close.focus());
  fireEvent.click(close);
  expect(document.activeElement).toBe(screen.getByRole("complementary", { name: "Files" }));
});

it("preserves outside focus when a main view is presented", () => {
  testClient.render(
    <>
      <button>Outside control</button>
      <ChatPanel onSend={() => true} />
    </>,
  );
  const outside = screen.getByRole("button", { name: "Outside control" });
  act(() => outside.focus());
  act(() => openWorkspaceView(testClient.workspace.access, "file"));
  expect(document.activeElement).toBe(outside);
});

describe("explicit Composer focus", () => {
  beforeEach(() => {
    model.views.push({
      id: "settings",
      title: "Settings",
      component: () => <button>Settings controls</button>,
    });
  });

  function mount() {
    const panel = createRef<ChatPanelHandle>();
    const view = testClient.render(
      <>
        <button>Later control</button>
        <ChatPanel ref={panel} onSend={() => true} />
      </>,
    );
    const draft = screen.getByLabelText<HTMLTextAreaElement>("Composer draft");
    return { panel, view, draft };
  }

  function settings() {
    openWorkspaceViewInDock(testClient.workspace.access, "file");
    openWorkspaceView(testClient.workspace.access, "settings");
    const result = mount();
    act(() => screen.getByRole("button", { name: "Settings controls" }).focus());
    return result;
  }

  it("reveals the existing draft and preserves the wide Dock and one-step history", () => {
    const { panel, draft } = settings();
    const previous = testClient.navigation.get();
    draft.setSelectionRange(5, 5);

    act(() => panel.current!.focusComposer());

    expect(screen.getByRole("region", { name: "Conversation" })).toBeTruthy();
    expect(screen.getByRole("complementary", { name: "Files" })).toBeTruthy();
    expect(document.activeElement).toBe(draft);
    expect(draft.value).toBe("Keep this draft — 保留");
    expect(draft.selectionEnd).toBe(5);
    expect(testClient.navigation.get()).toMatchObject({ view: null, dock: "file" });
    act(() => testClient.navigation.back());
    expect(testClient.navigation.get()).toEqual(previous);
    expect(screen.getByRole("region", { name: "Settings" })).toBeTruthy();
  });

  it("focuses a visible draft without navigating or hiding the Dock", () => {
    openWorkspaceViewInDock(testClient.workspace.access, "file");
    const { panel, draft } = mount();
    const previous = testClient.navigation.get();
    act(() => screen.getByRole("button", { name: "Other control" }).focus());

    act(() => panel.current!.focusComposer());

    expect(document.activeElement).toBe(draft);
    expect(testClient.navigation.get()).toBe(previous);
    expect(screen.getByRole("complementary", { name: "Files" })).toBeTruthy();
  });

  it("reveals the conversation from a narrow inspection and retains its source and material", () => {
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(700);
    const { panel, draft } = mount();
    const origin = screen.getByRole("button", { name: "src/main.ts:4" });
    act(() => origin.focus());
    fireEvent.click(origin);
    const controls = screen.getByRole("button", { name: "File controls" });
    const previous = testClient.navigation.get();
    act(() => controls.focus());

    act(() => panel.current!.focusComposer());

    expect(testClient.navigation.get()).toMatchObject({ view: null, dock: null });
    expect(screen.getByRole("region", { name: "Conversation" })).toBeTruthy();
    expect(document.activeElement).toBe(draft);
    expect(controls.isConnected).toBe(true);
    act(() => testClient.navigation.back());
    expect(testClient.navigation.get()).toEqual(previous);
    expect(screen.getByRole("button", { name: "File controls" })).toBe(controls);
    expect(screen.getByRole("button", { name: "Back to conversation" })).toBeTruthy();
    expect(draft.value).toBe("Keep this draft — 保留");
  });

  it("manually reopens the inspected material after focusing a narrow conversation", () => {
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(700);
    openWorkspaceViewInDock(testClient.workspace.access, "subagents");
    const { panel, draft } = mount();
    const origin = screen.getByRole("button", { name: "src/main.ts:4" });
    act(() => origin.focus());
    fireEvent.click(origin);
    const controls = screen.getByRole("button", { name: "File controls" });

    act(() => panel.current!.focusComposer());
    expect(document.activeElement).toBe(draft);
    fireEvent.click(screen.getByRole("button", { name: /Open material full width/ }));

    expect(screen.getByRole("button", { name: "File controls" })).toBe(controls);
    expect(screen.getByRole("button", { name: "Back to conversation" })).toBeTruthy();
    expect(draft.value).toBe("Keep this draft — 保留");
  });

  it.each(["control", "conversation"])("keeps later focus on a %s", (target) => {
    const { panel } = settings();
    const later =
      target === "control"
        ? screen.getByRole("button", { name: "Later control" })
        : screen.getByLabelText("Conversation");

    act(() => {
      panel.current!.focusComposer();
      later.focus();
    });

    expect(testClient.navigation.get().view).toBeNull();
    expect(document.activeElement).toBe(later);
  });

  it("retires the request when the Session changes before delivery", () => {
    const { panel, view, draft } = settings();

    act(() => {
      panel.current!.focusComposer();
      model.sessionId = "ses_second";
      view.rerender(
        <>
          <button>Later control</button>
          <ChatPanel ref={panel} onSend={() => true} />
        </>,
      );
    });

    expect(screen.getByLabelText("Composer draft")).toBe(draft);
    expect(document.activeElement).not.toBe(draft);
  });

  it.each(["file", "settings"])(
    "retires navigation superseded by %s without a later ghost focus",
    (viewId) => {
      const { panel, draft } = settings();

      act(() => {
        panel.current!.focusComposer();
        openWorkspaceView(testClient.workspace.access, viewId);
      });

      expect(testClient.navigation.get().view).toBe(viewId);
      const close = within(windowHeader()!).getByRole("button", { name: "Close" });
      act(() => close.focus());
      fireEvent.click(close);
      expect(document.activeElement).not.toBe(draft);
      expect(document.activeElement).toBe(screen.getByRole("complementary", { name: "Files" }));
    },
  );

  it("keeps an open modal's focus and underlying presentation", () => {
    const { panel } = settings();
    testClient.render(
      <div role="dialog" aria-label="Editing task">
        <button>Modal control</button>
      </div>,
    );
    const control = screen.getByRole("button", { name: "Modal control" });
    act(() => control.focus());
    const previous = testClient.navigation.get();

    act(() => panel.current!.focusComposer());

    expect(document.activeElement).toBe(control);
    expect(testClient.navigation.get()).toBe(previous);
  });
});

describe("draft intents in workspace presentations", () => {
  function mount(width = 700) {
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(width);
    const content = (
      <>
        <button>Later draft control</button>
        <ChatPanel onSend={() => true} />
      </>
    );
    const view = testClient.render(content);
    const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Composer draft" });
    return { view, draft };
  }

  function inspectChild() {
    const origin = screen.getByRole("button", { name: "Inspect task" });
    act(() => origin.focus());
    fireEvent.click(origin);
    const evidence = screen.getByRole("button", { name: "Evidence 0" });
    fireEvent.click(evidence);
    act(() => evidence.focus());
    return evidence;
  }

  async function openMessageMenu(text: string) {
    fireEvent.contextMenu(await screen.findByText(text));
    await act(async () => drainBrowserTasks());
    return screen.getByRole("menu", { name: "You" });
  }

  async function chooseMessageAction(label: string) {
    fireEvent.click(screen.getByRole("menuitem", { name: label }));
    await act(async () => drainBrowserTasks());
  }

  function visibleDraft(original: HTMLTextAreaElement, value: string) {
    const conversation = screen.getByRole("region", { name: "Conversation" });
    const draft = within(conversation).getByRole<HTMLTextAreaElement>("textbox", {
      name: "Composer draft",
    });
    expect(draft).toBe(original);
    expect(draft.value).toBe(value);
    expect(document.activeElement).toBe(draft);
    expect(draft.selectionStart).toBe(value.length);
    expect(draft.selectionEnd).toBe(value.length);
  }

  async function editChild(width: number) {
    const { draft } = mount(width);
    const evidence = inspectChild();
    const previous = testClient.navigation.get();
    await openMessageMenu(CHILD_TEXT);

    await chooseMessageAction("Edit in composer");

    expect(screen.queryByRole("menu")).toBeNull();
    visibleDraft(draft, CHILD_TEXT);
    expect(evidence.isConnected).toBe(true);
    expect(model.rollback).not.toHaveBeenCalled();
    fireEvent.change(draft, { target: { value: `${CHILD_TEXT} Continue here.` } });
    expect(captureComposerSubmission(testClient.composer.access).value).toBe(
      `${CHILD_TEXT} Continue here.`,
    );
    expect(document.activeElement).toBe(draft);
    return { draft, evidence, previous };
  }

  it("edits a child through the shared menu into the narrow Composer and retains its material history", async () => {
    const { draft, evidence, previous } = await editChild(700);

    expect(testClient.navigation.get()).toMatchObject({ view: null, dock: null });
    act(() => testClient.navigation.back());
    expect(testClient.navigation.get()).toEqual(previous);
    expect(screen.getByRole("region", { name: "Subagents" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Evidence 1" })).toBe(evidence);
    expect(screen.getByRole("button", { name: "Back to conversation" })).toBeTruthy();
    expect(draft.value).toBe(`${CHILD_TEXT} Continue here.`);
  });

  it("edits a child through the shared menu into the wide Composer without hiding its Dock", async () => {
    const { draft, evidence, previous } = await editChild(1200);

    expect(testClient.navigation.get()).toBe(previous);
    expect(screen.getByRole("complementary", { name: "Subagents" }).contains(evidence)).toBe(true);
    expect(draft.value).toBe(`${CHILD_TEXT} Continue here.`);
  });

  it.each(["Copy plain text", "Escape"])(
    "keeps a narrow inspection and draft when the shared menu closes with %s",
    async (action) => {
      const { draft } = mount();
      const evidence = inspectChild();
      const previous = testClient.navigation.get();
      const menu = await openMessageMenu(CHILD_TEXT);

      if (action === "Escape") {
        fireEvent.keyDown(menu, { key: "Escape" });
        await act(async () => drainBrowserTasks());
      } else {
        await chooseMessageAction(action);
      }

      expect(testClient.navigation.get()).toBe(previous);
      expect(screen.queryByRole("region", { name: "Conversation" })).toBeNull();
      expect(screen.queryByRole("textbox", { name: "Composer draft" })).toBeNull();
      expect(draft.value).toBe(INITIAL_DRAFT);
      expect(document.activeElement).toBe(evidence);
      expect(writeToClipboard).toHaveBeenCalledTimes(action === "Escape" ? 0 : 1);
      expect(model.rollback).not.toHaveBeenCalled();
    },
  );

  it("reveals the narrow Composer when quoted text is appended", () => {
    const { draft } = mount();
    const evidence = inspectChild();

    act(() =>
      appendToComposerDraft(testClient.composer.access, "> Evidence selected from the child task."),
    );

    visibleDraft(draft, `${INITIAL_DRAFT}\n\n> Evidence selected from the child task.`);
    expect(evidence.isConnected).toBe(true);
    expect(testClient.navigation.get()).toMatchObject({ view: null, dock: null });
  });

  it("delivers the accepted input's caret when a history edit completes before reveal", () => {
    const { draft } = mount();
    inspectChild();
    let accepted = false;

    act(() => {
      const replace = beginComposerDraftReplacement(testClient.composer.access);
      accepted = replace({ text: "Canonical input committed before presentation" });
    });

    expect(accepted).toBe(true);
    visibleDraft(draft, "Canonical input committed before presentation");
  });

  it("does not leave later material or reclaim its focus when a root history edit completes", async () => {
    const rollback = Promise.withResolvers<{
      status: "committed";
      userInput: { parts: [{ kind: "text"; text: string }] };
    }>();
    model.rollback.mockReturnValue(rollback.promise);
    const { draft } = mount();
    await openMessageMenu("Revise the root task input.");
    await chooseMessageAction("Edit & rerun from here");
    expect(document.activeElement).toBe(draft);
    const file = screen.getByRole("button", { name: "src/main.ts:4" });
    act(() => file.focus());
    fireEvent.click(file);
    const control = screen.getByRole("button", { name: "File controls" });
    act(() => control.focus());
    const previous = testClient.navigation.get();

    await act(async () => {
      rollback.resolve({
        status: "committed",
        userInput: { parts: [{ kind: "text", text: "Delayed canonical input" }] },
      });
      await rollback.promise;
    });

    expect(testClient.navigation.get()).toBe(previous);
    expect(screen.getByRole("region", { name: "Files" })).toBeTruthy();
    expect(screen.queryByRole("textbox", { name: "Composer draft" })).toBeNull();
    expect(screen.getByLabelText("Composer draft")).toBe(draft);
    expect(draft.value).toBe("Delayed canonical input");
    expect(document.activeElement).toBe(control);
    expect(model.rollback.mock.calls).toEqual([["ses_first", "root-run", "history"]]);
  });

  it("keeps focus moved after a draft intent but before its reveal", () => {
    const { draft } = mount();
    inspectChild();
    const later = screen.getByRole("button", { name: "Later draft control" });

    act(() => {
      replaceComposerDraft(testClient.composer.access, { text: "Edited child task" });
      later.focus();
    });

    expect(screen.getByRole("region", { name: "Conversation" })).toBeTruthy();
    expect(draft.value).toBe("Edited child task");
    expect(document.activeElement).toBe(later);
  });

  it("keeps modal focus and presentation when a draft intent reaches the reveal owner", () => {
    const { draft } = mount();
    inspectChild();
    testClient.render(
      <div role="dialog" aria-label="Editing task">
        <button>Modal draft control</button>
      </div>,
    );
    const control = screen.getByRole("button", { name: "Modal draft control" });
    act(() => control.focus());
    const previous = testClient.navigation.get();

    act(() =>
      replaceComposerDraft(testClient.composer.access, { text: "Edited draft beneath the modal" }),
    );

    expect(testClient.navigation.get()).toBe(previous);
    expect(screen.queryByRole("textbox", { name: "Composer draft" })).toBeNull();
    expect(draft.value).toBe("Edited draft beneath the modal");
    expect(document.activeElement).toBe(control);
  });

  it("retires draft focus when the Session changes before reveal", () => {
    const { view, draft } = mount();
    inspectChild();

    act(() => {
      replaceComposerDraft(testClient.composer.access, { text: "Edited first Session" });
      model.sessionId = "ses_second";
      testClient.agent.access.selectAgentSession(model.sessionId);
      view.rerender(
        <>
          <button>Later draft control</button>
          <ChatPanel onSend={() => true} />
        </>,
      );
    });

    expect(screen.getByLabelText("Composer draft")).toBe(draft);
    expect(document.activeElement).not.toBe(draft);
    expect(captureComposerSubmission(testClient.composer.access).value).not.toBe(
      "Edited first Session",
    );
  });

  it("retires draft focus when later navigation supersedes reveal", () => {
    const { draft } = mount();
    inspectChild();

    act(() => {
      replaceComposerDraft(testClient.composer.access, { text: "Edited child task" });
      openWorkspaceView(testClient.workspace.access, "file");
    });

    expect(screen.getByRole("region", { name: "Files" })).toBeTruthy();
    expect(document.activeElement).not.toBe(draft);
    const close = within(windowHeader()!).getByRole("button", { name: "Close" });
    act(() => close.focus());
    fireEvent.click(close);
    expect(screen.getByRole("region", { name: "Conversation" })).toBeTruthy();
    expect(document.activeElement).not.toBe(draft);
  });
});

describe("main presentation focus handoff", () => {
  beforeEach(() => {
    model.views.push({
      id: "settings",
      title: "Settings",
      component: () => <button>Settings controls</button>,
    });
  });

  function materialHistory(width: number) {
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(width);
    openWorkspaceViewInDock(testClient.workspace.access, "file");
    openWorkspaceView(testClient.workspace.access, "file");
    testClient.render(
      <>
        <button>Later navigation focus</button>
        <ChatPanel onSend={() => true} />
      </>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    const draft = screen.getByLabelText<HTMLTextAreaElement>("Composer draft");
    act(() => draft.focus());
    draft.setSelectionRange(4, 4);
    act(() => testClient.navigation.back());
    const material = screen.getByRole("region", { name: "Files" });
    expect(document.activeElement).toBe(material);
    return { draft, material, control: screen.getByRole("button", { name: "File controls" }) };
  }

  it.each(["region", "child"])(
    "hands hidden retained material %s focus to the narrow conversation on history forward",
    (origin) => {
      const { draft, material, control } = materialHistory(700);
      if (origin === "child") act(() => control.focus());

      act(() => testClient.navigation.forward());

      expect(testClient.navigation.get().view).toBeNull();
      expect(material.isConnected).toBe(true);
      expect(control.isConnected).toBe(true);
      expect(document.activeElement).toBe(screen.getByRole("region", { name: "Conversation" }));
      expect(screen.getByLabelText("Composer draft")).toBe(draft);
      expect(draft.value).toBe("Keep this draft — 保留");
      expect(draft.selectionEnd).toBe(4);
    },
  );

  it("keeps the same material control focused when history returns it to a visible Dock", () => {
    const { control } = materialHistory(1200);
    act(() => control.focus());

    act(() => testClient.navigation.forward());

    expect(screen.getByRole("complementary", { name: "Files" }).contains(control)).toBe(true);
    expect(document.activeElement).toBe(control);
  });

  it("preserves later outside focus when history hides a narrow material", () => {
    materialHistory(700);
    const later = screen.getByRole("button", { name: "Later navigation focus" });

    act(() => {
      testClient.navigation.forward();
      later.focus();
    });

    expect(screen.getByRole("region", { name: "Conversation" })).toBeTruthy();
    expect(document.activeElement).toBe(later);
  });

  it("enters a main view when focus belonged to the conversation it hides", () => {
    testClient.render(<ChatPanel onSend={() => true} />);
    const control = screen.getByRole("button", { name: "Other control" });
    act(() => control.focus());
    act(() => openWorkspaceView(testClient.workspace.access, "settings"));

    expect(screen.queryByRole("region", { name: "Conversation" })).toBeNull();
    expect(control.isConnected).toBe(true);
    expect(document.activeElement).toBe(screen.getByRole("region", { name: "Settings" }));
  });

  it("enters a different main view from a retained Dock child", () => {
    openWorkspaceViewInDock(testClient.workspace.access, "subagents");
    testClient.render(<ChatPanel onSend={() => true} />);
    const child = screen.getByRole("button", { name: "Evidence 0" });
    fireEvent.click(child);
    act(() => child.focus());
    act(() => openWorkspaceView(testClient.workspace.access, "settings"));

    expect(child.isConnected).toBe(true);
    expect(screen.queryByRole("button", { name: "Evidence 1" })).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("region", { name: "Settings" }));

    fireEvent.click(within(windowHeader()!).getByRole("button", { name: "Close" }));
    expect(screen.getByRole("button", { name: "Evidence 1" })).toBe(child);
  });

  it("enters a main view from the retained panel catalog without losing its query", () => {
    showWorkspaceDock(testClient.workspace.access);
    testClient.render(<ChatPanel onSend={() => true} />);
    const search = screen.getByRole<HTMLInputElement>("combobox", { name: "Find a panel…" });
    fireEvent.change(search, { target: { value: "Files" } });
    act(() => search.focus());
    act(() => openWorkspaceView(testClient.workspace.access, "settings"));

    expect(search.isConnected).toBe(true);
    expect(screen.queryByRole("combobox", { name: "Find a panel…" })).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("region", { name: "Settings" }));

    fireEvent.click(within(windowHeader()!).getByRole("button", { name: "Close" }));
    expect(screen.getByRole("combobox", { name: "Find a panel…" })).toBe(search);
    expect(search.value).toBe("Files");
  });

  it("keeps a child that stays visible while its material moves into the Dock", () => {
    openWorkspaceView(testClient.workspace.access, "file");
    testClient.render(<ChatPanel onSend={() => true} />);
    const controls = screen.getByRole("button", { name: "File controls" });
    act(() => controls.focus());
    act(() => openWorkspaceViewInDock(testClient.workspace.access, "file"));

    expect(testClient.navigation.get().view).toBeNull();
    expect(screen.getByRole("complementary", { name: "Files" }).contains(controls)).toBe(true);
    expect(document.activeElement).toBe(controls);
  });

  it("does not replay the handoff while the same main view updates", () => {
    openWorkspaceView(testClient.workspace.access, "settings");
    const view = testClient.render(<ChatPanel onSend={() => true} />);
    const controls = screen.getByRole("button", { name: "Settings controls" });
    act(() => controls.focus());
    view.rerender(<ChatPanel onSend={() => true} />);

    expect(document.activeElement).toBe(controls);
  });
});

describe("Dock collapse focus handoff", () => {
  it("associates the selected Dock tab with its retained material viewport", () => {
    openWorkspaceViewInDock(testClient.workspace.access, "subagents");
    testClient.render(<ChatPanel onSend={() => true} />);
    const tab = screen.getByRole("tab", { name: "Subagents" });
    const panel = screen.getByRole("tabpanel", { name: "Subagents" });

    expect(tab.getAttribute("aria-controls")).toBe(panel.id);
    expect(panel.getAttribute("aria-labelledby")).toBe(tab.id);
    expect(tab.getAttribute("aria-selected")).toBe("true");
    expect(panel.contains(screen.getByRole("button", { name: "Evidence 0" }))).toBe(true);
  });

  it("leaves material tabs unselected while the catalog is presented", () => {
    openWorkspaceViewInDock(testClient.workspace.access, "subagents");
    openWorkspaceViewInDock(testClient.workspace.access, "catalog");
    testClient.render(<ChatPanel onSend={() => true} />);

    expect(screen.queryByRole("tab", { selected: true })).toBeNull();
    expect(screen.queryByRole("tabpanel")).toBeNull();
    expect(screen.getByRole("combobox", { name: "Find a panel…" })).toBeTruthy();
  });

  it("preserves the filtered catalog when it expands and returns beside the conversation", () => {
    openWorkspaceViewInDock(testClient.workspace.access, "subagents");
    openWorkspaceViewInDock(testClient.workspace.access, "catalog");
    testClient.render(<ChatPanel onSend={() => true} />);
    const search = screen.getByRole<HTMLInputElement>("combobox", { name: "Find a panel…" });
    fireEvent.change(search, { target: { value: "Files" } });
    fireEvent.click(screen.getByRole("button", { name: "Open full width" }));

    expect(testClient.navigation.get().view).toBe("catalog");
    expect(screen.getByRole("combobox", { name: "Find a panel…" })).toBe(search);
    expect(search.value).toBe("Files");
    fireEvent.click(screen.getByRole("button", { name: "Open in the context dock" }));

    expect(testClient.navigation.get().view).toBeNull();
    expect(screen.getByRole("combobox", { name: "Find a panel…" })).toBe(search);
    expect(search.value).toBe("Files");
    expect(screen.queryByRole("tab", { selected: true })).toBeNull();
    expect(screen.queryByRole("button", { name: "Back to panel list" })).toBeNull();
  });

  it("expands and restores the same reader without replacing its inspection source", () => {
    openWorkspaceViewInDock(testClient.workspace.access, "subagents");
    testClient.render(<ChatPanel onSend={() => true} />);
    fireEvent.click(screen.getByRole("button", { name: "Evidence 0" }));
    const source = screen.getByRole("button", { name: "src/child.ts:8" });
    act(() => source.focus());
    fireEvent.click(source);
    const viewport = screen.getByRole("tabpanel", { name: "Files" });
    viewport.scrollTop = 241;
    fireEvent.scroll(viewport);
    const expand = screen.getByRole("button", { name: "Open full width" });
    act(() => expand.focus());
    fireEvent.click(expand);

    expect(testClient.navigation.get().view).toBe("file");
    expect(screen.getByRole("region", { name: "Files" })).toBe(viewport);
    expect(viewport.scrollTop).toBe(241);
    expect(document.activeElement).toBe(viewport);
    expect(screen.getByRole("button", { name: "Back to Subagents" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Open in the context dock" }));

    expect(testClient.navigation.get().view).toBeNull();
    expect(screen.getByRole("tabpanel", { name: "Files" })).toBe(viewport);
    expect(viewport.scrollTop).toBe(241);
    expect(screen.queryByRole("button", { name: "Back to Files" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Back to Subagents" }));

    expect(document.activeElement).toBe(source);
    expect(screen.getByRole("button", { name: "Evidence 1" })).toBeTruthy();
    expect(screen.getByLabelText<HTMLTextAreaElement>("Composer draft").value).toBe(INITIAL_DRAFT);
  });

  it("keeps inactive workspace scrollports laid out but inert until their tab returns", () => {
    openWorkspaceViewInDock(testClient.workspace.access, "subagents");
    testClient.render(<ChatPanel onSend={() => true} />);
    fireEvent.click(screen.getByRole("button", { name: "Evidence 0" }));
    const viewport = screen.getByRole("tabpanel", { name: "Subagents" });
    const retained = viewport.closest<HTMLElement>("[data-dock-view-id]")!;
    viewport.scrollTop = 241;
    fireEvent.scroll(viewport);

    act(() => openWorkspaceViewInDock(testClient.workspace.access, "file"));
    expect(viewport.isConnected).toBe(true);
    expect(retained.hidden).toBe(false);
    expect(retained.hasAttribute("inert")).toBe(true);
    expect(screen.queryByRole("tabpanel", { name: "Subagents" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Evidence 1" })).toBeNull();

    act(() => openWorkspaceViewInDock(testClient.workspace.access, "subagents"));
    expect(screen.getByRole("tabpanel", { name: "Subagents" })).toBe(viewport);
    expect(screen.getByRole("button", { name: "Evidence 1" })).toBeTruthy();
    expect(retained.hasAttribute("inert")).toBe(false);
    expect(viewport.scrollTop).toBe(241);
  });

  function retainedDock() {
    openWorkspaceViewInDock(testClient.workspace.access, "subagents");
    testClient.render(
      <>
        <button>Later control</button>
        <ChatPanel onSend={() => true} />
      </>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Evidence 0" }));
    const draft = screen.getByLabelText<HTMLTextAreaElement>("Composer draft");
    draft.setSelectionRange(5, 5);
    return {
      dock: screen.getByRole("complementary", { name: "Subagents" }),
      child: screen.getByRole("button", { name: "Evidence 1" }),
      draft,
    };
  }

  function expectRetainedDock(dock: HTMLElement, child: HTMLElement, draft: HTMLTextAreaElement) {
    expect(screen.getByRole("complementary", { name: "Subagents" })).toBe(dock);
    expect(screen.getByRole("button", { name: "Evidence 1" })).toBe(child);
    expect(screen.getByLabelText("Composer draft")).toBe(draft);
    expect(draft.value).toBe("Keep this draft — 保留");
    expect(draft.selectionEnd).toBe(5);
  }

  it.each(["region", "child"])(
    "hands retained Dock %s focus to the conversation on collapse and keeps it on reopen",
    (origin) => {
      const { dock, child, draft } = retainedDock();
      act(() => (origin === "region" ? dock : child).focus());

      act(() => toggleWorkspaceDock(testClient.workspace.access));

      const conversation = screen.getByRole("region", { name: "Conversation" });
      expect(document.activeElement).toBe(conversation);
      expect(dock.isConnected).toBe(true);
      expect(child.isConnected).toBe(true);

      act(() => toggleWorkspaceDock(testClient.workspace.access));

      expectRetainedDock(dock, child, draft);
      expect(document.activeElement).toBe(conversation);
    },
  );

  it("keeps a focused Composer and its selection through collapse and reopen", () => {
    const { dock, child, draft } = retainedDock();
    act(() => draft.focus());

    act(() => toggleWorkspaceDock(testClient.workspace.access));
    expect(document.activeElement).toBe(draft);

    act(() => toggleWorkspaceDock(testClient.workspace.access));
    expect(document.activeElement).toBe(draft);
    expectRetainedDock(dock, child, draft);
  });

  it("keeps the visible toggle focused while it collapses and shows the Dock", () => {
    const { dock, child, draft } = retainedDock();
    const toggle = screen.getByRole("button", { name: t("dock.action.hide") });
    act(() => toggle.focus());

    fireEvent.click(toggle);
    expect(toggle.getAttribute("aria-label")).toBe(t("dock.action.show"));
    expect(document.activeElement).toBe(toggle);

    fireEvent.click(toggle);
    expect(toggle.getAttribute("aria-label")).toBe(t("dock.action.hide"));
    expect(document.activeElement).toBe(toggle);
    expectRetainedDock(dock, child, draft);
  });

  it("preserves later outside focus when the Dock collapses", () => {
    const { child } = retainedDock();
    const later = screen.getByRole("button", { name: "Later control" });
    act(() => child.focus());

    act(() => {
      toggleWorkspaceDock(testClient.workspace.access);
      later.focus();
    });

    expect(document.activeElement).toBe(later);
  });

  it("keeps the visible catalog entry focused after closing the last Dock tab settles", async () => {
    const { child, draft } = retainedDock();
    const tab = screen.getByRole("tab", { name: "Subagents" });
    act(() => tab.focus());

    fireEvent.keyDown(tab, { key: "Delete" });
    await act(async () => {
      await new Promise((resolve) => requestAnimationFrame(resolve));
    });

    expect(child.isConnected).toBe(false);
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: t("dock.action.browse") }),
    );
    expect(screen.getByLabelText("Composer draft")).toBe(draft);
    expect(draft.value).toBe("Keep this draft — 保留");
    expect(draft.selectionEnd).toBe(5);
  });

  it("hands Dock focus to the conversation when the row narrows below the Dock", async () => {
    const resize = observeRowWidth(1200);
    const { child, draft } = retainedDock();
    act(() => child.focus());

    await resize(700);

    expect(testClient.navigation.get().view).toBeNull();
    expect(windowHeader()).toBeNull();
    const conversation = screen.getByRole("region", { name: "Conversation" });
    expect(document.activeElement).toBe(conversation);
    expect(child.isConnected).toBe(true);

    await resize(1200);
    expect(screen.getByRole("button", { name: "Evidence 1" })).toBe(child);
    expect(document.activeElement).toBe(conversation);
    expect(screen.getByLabelText("Composer draft")).toBe(draft);
  });

  async function settleCleanup() {
    await act(async () => {
      await new Promise((resolve) => requestAnimationFrame(resolve));
    });
    await settle();
  }

  async function openPanelPicker() {
    fireEvent.click(screen.getByRole("button", { name: t("dock.action.browse") }));
    const search = await screen.findByRole("combobox", { name: "Find a panel…" });
    await vi.waitFor(() => expect(document.activeElement).toBe(search));
    return search;
  }

  it("retires an open panel picker on collapse and reveals the Dock without it", async () => {
    const { dock, child, draft } = retainedDock();
    const search = await openPanelPicker();

    act(() => toggleWorkspaceDock(testClient.workspace.access));
    await settleCleanup();

    expect(search.isConnected).toBe(false);
    const conversation = screen.getByRole("region", { name: "Conversation" });
    expect(document.activeElement).toBe(conversation);
    fireEvent.keyDown(conversation, { key: "Escape" });
    expect(document.activeElement).toBe(conversation);

    act(() => toggleWorkspaceDock(testClient.workspace.access));
    await settleCleanup();

    expect(screen.queryByRole("combobox", { name: "Find a panel…" })).toBeNull();
    expectRetainedDock(dock, child, draft);
    expect(document.activeElement).toBe(conversation);
  });

  it("retires an open tab menu on collapse and hands focus to the conversation", async () => {
    const { dock, child, draft } = retainedDock();
    const tab = screen.getByRole("tab", { name: "Subagents" });
    act(() => tab.focus());
    fireEvent.contextMenu(tab);
    const menu = await screen.findByRole("menu");
    await vi.waitFor(() => expect(menu.contains(document.activeElement)).toBe(true));

    act(() => toggleWorkspaceDock(testClient.workspace.access));
    await settleCleanup();

    expect(menu.isConnected).toBe(false);
    expect(screen.queryByRole("menu")).toBeNull();
    const conversation = screen.getByRole("region", { name: "Conversation" });
    expect(document.activeElement).toBe(conversation);
    fireEvent.keyDown(conversation, { key: "Escape" });
    expect(document.activeElement).toBe(conversation);

    act(() => toggleWorkspaceDock(testClient.workspace.access));
    await settleCleanup();

    expect(screen.queryByRole("menu")).toBeNull();
    expectRetainedDock(dock, child, draft);
  });

  it("preserves later outside focus while an open panel picker retires on collapse", async () => {
    retainedDock();
    const later = screen.getByRole("button", { name: "Later control" });
    const search = await openPanelPicker();

    act(() => {
      toggleWorkspaceDock(testClient.workspace.access);
      later.focus();
    });
    await settleCleanup();

    expect(search.isConnected).toBe(false);
    expect(document.activeElement).toBe(later);
  });

  it("retires an open panel picker when the row narrows below the Dock", async () => {
    const resize = observeRowWidth(1200);
    const { child, draft } = retainedDock();
    const search = await openPanelPicker();

    await resize(700);
    await settleCleanup();

    expect(search.isConnected).toBe(false);
    expect(testClient.navigation.get().view).toBeNull();
    expect(windowHeader()).toBeNull();
    const conversation = screen.getByRole("region", { name: "Conversation" });
    expect(document.activeElement).toBe(conversation);

    await resize(1200);
    await settleCleanup();
    expect(screen.queryByRole("combobox", { name: "Find a panel…" })).toBeNull();
    expect(screen.getByRole("button", { name: "Evidence 1" })).toBe(child);
    expect(document.activeElement).toBe(conversation);
    expect(screen.getByLabelText("Composer draft")).toBe(draft);
    expect(draft.selectionEnd).toBe(5);
  });

  describe("material popups", () => {
    type MaterialPopup = "popover" | "context-menu";

    function MaterialOptions({ popup }: { popup: MaterialPopup }) {
      return popup === "popover" ? (
        <Popover.Root>
          <Popover.Trigger>Material options</Popover.Trigger>
          <Popover.Content aria-label="Material panel">
            <input aria-label="Material line" />
          </Popover.Content>
        </Popover.Root>
      ) : (
        <ContextMenu.Root>
          <ContextMenu.Trigger render={<button>Material options</button>} />
          <ContextMenu.Content>
            <ContextMenu.IconItem icon="copy" onSelect={() => {}}>
              Copy path
            </ContextMenu.IconItem>
          </ContextMenu.Content>
        </ContextMenu.Root>
      );
    }

    function retainedPopupDock(popup: MaterialPopup) {
      model.views[1] = {
        ...model.views[1]!,
        component: () => (
          <>
            <ChildReader />
            <MaterialOptions popup={popup} />
          </>
        ),
      };
      return retainedDock();
    }

    async function openMaterialPopup(popup: MaterialPopup) {
      const trigger = screen.getByRole("button", { name: "Material options" });
      act(() => trigger.focus());
      if (popup === "popover") fireEvent.click(trigger);
      else fireEvent.contextMenu(trigger);
      const opened = await screen.findByRole(popup === "popover" ? "dialog" : "menu");
      await vi.waitFor(() => expect(opened.contains(document.activeElement)).toBe(true));
      return opened;
    }

    it.each(["popover", "context-menu"] as const)(
      "retires a material %s on collapse and reveals the Dock without it",
      async (popup) => {
        const { dock, child, draft } = retainedPopupDock(popup);
        const opened = await openMaterialPopup(popup);

        act(() => toggleWorkspaceDock(testClient.workspace.access));
        await settleCleanup();

        expect(opened.isConnected).toBe(false);
        const conversation = screen.getByRole("region", { name: "Conversation" });
        expect(document.activeElement).toBe(conversation);

        act(() => toggleWorkspaceDock(testClient.workspace.access));
        await settleCleanup();

        expect(screen.queryByRole(popup === "popover" ? "dialog" : "menu")).toBeNull();
        expectRetainedDock(dock, child, draft);
        expect(document.activeElement).toBe(conversation);
      },
    );

    it("retires a material popover when the row narrows below the Dock", async () => {
      const resize = observeRowWidth(1200);
      const { dock, child, draft } = retainedPopupDock("popover");
      const opened = await openMaterialPopup("popover");

      await resize(700);
      await settleCleanup();

      expect(opened.isConnected).toBe(false);
      expect(windowHeader()).toBeNull();
      const conversation = screen.getByRole("region", { name: "Conversation" });
      expect(document.activeElement).toBe(conversation);

      await resize(1200);
      await settleCleanup();

      expect(screen.queryByRole("dialog")).toBeNull();
      expectRetainedDock(dock, child, draft);
      expect(document.activeElement).toBe(conversation);
    });
  });
});

it("retains a main-only source while inspecting a file and restores the same source on return", () => {
  function SourceMaterial() {
    const [query, setQuery] = useState("");
    return (
      <>
        <SearchField
          aria-label="Main material query"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
        />
        <FileRefLink path="src/full-source.ts" line={8} />
      </>
    );
  }
  model.views.push({ id: "main-source", title: "Source material", component: SourceMaterial });
  openWorkspaceView(testClient.workspace.access, "main-source");
  testClient.render(<ChatPanel onSend={() => true} />);
  const query = screen.getByRole<HTMLInputElement>("searchbox", { name: "Main material query" });
  fireEvent.change(query, { target: { value: "retained source" } });
  const origin = screen.getByRole("button", { name: "src/full-source.ts:8" });
  act(() => origin.focus());
  fireEvent.click(origin);
  expect(testClient.navigation.get().view).toBeNull();
  expect(query.isConnected).toBe(true);
  expect(screen.queryByRole("searchbox", { name: "Main material query" })).toBeNull();

  const file = screen.getByRole("button", { name: "File controls" });
  act(() => file.focus());
  fireEvent.keyDown(file, { key: "Escape" });
  expect(testClient.navigation.get().view).toBe("main-source");
  expect(screen.getByRole("searchbox", { name: "Main material query" })).toBe(query);
  expect(query.value).toBe("retained source");
  expect(document.activeElement).toBe(origin);
});

it("retires a moved main view when its Dock tab is closed", () => {
  model.views.push({
    id: "moved-source",
    title: "Moved material",
    dock: "workspace",
    component: () => <input aria-label="Moved material draft" defaultValue="draft" />,
  });
  openWorkspaceView(testClient.workspace.access, "moved-source");
  testClient.render(<ChatPanel onSend={() => true} />);
  const draft = screen.getByRole("textbox", { name: "Moved material draft" });
  fireEvent.click(screen.getByRole("button", { name: "Open in the context dock" }));
  expect(testClient.navigation.get().view).toBeNull();
  expect(draft.isConnected).toBe(true);
  fireEvent.keyDown(screen.getByRole("tab", { name: "Moved material" }), { key: "Delete" });
  expect(draft.isConnected).toBe(false);
});

describe("material inspection focus", () => {
  it("clears the local search before returning an inspected material to its source", () => {
    model.views[0] = { ...model.views[0]!, component: FilteredMaterial };
    testClient.render(<ChatPanel onSend={() => true} />);
    const source = screen.getByRole("button", { name: "src/main.ts:4" });
    act(() => source.focus());
    fireEvent.click(source);
    const filter = screen.getByRole<HTMLInputElement>("searchbox", { name: "Filter material" });
    act(() => filter.focus());
    fireEvent.keyDown(filter, { key: "Escape" });
    expect(filter.value).toBe("");
    expect(document.activeElement).toBe(filter);
    expect(screen.getByRole("button", { name: "Back to conversation" })).toBeTruthy();
    fireEvent.keyDown(filter, { key: "Escape" });
    expect(screen.queryByRole("button", { name: "Back to conversation" })).toBeNull();
    expect(document.activeElement).toBe(source);
  });

  it.each([{ isComposing: true }, { keyCode: 229 }])(
    "preserves the active search and inspection during IME cancellation %o",
    (composition) => {
      model.views[0] = { ...model.views[0]!, component: FilteredMaterial };
      testClient.render(<ChatPanel onSend={() => true} />);
      fireEvent.click(screen.getByRole("button", { name: "src/main.ts:4" }));
      const filter = screen.getByRole<HTMLInputElement>("searchbox", { name: "Filter material" });
      act(() => filter.focus());
      fireEvent.keyDown(filter, { key: "Escape", ...composition });
      expect(filter.value).toBe("src/main");
      expect(document.activeElement).toBe(filter);
      expect(screen.getByRole("button", { name: "Back to conversation" })).toBeTruthy();
    },
  );

  it("returns through nested task and file inspections to the original conversation control", () => {
    testClient.render(<ChatPanel onSend={() => true} />);
    const source = screen.getByRole("button", { name: "Inspect task" });
    act(() => source.focus());
    fireEvent.click(source);
    expect(testClient.navigation.get().subagent).toBe("child");
    expect(screen.getByRole("button", { name: "Back to conversation" })).toBeTruthy();
    const nested = screen.getByRole("button", { name: "Inspect nested task" });
    act(() => nested.focus());
    fireEvent.click(nested);
    expect(testClient.navigation.get().subagent).toBe("nested");
    const file = screen.getByRole("button", { name: "src/child.ts:8" });
    act(() => file.focus());
    fireEvent.click(file);
    fireEvent.click(screen.getByRole("button", { name: "Back to task" }));
    expect(testClient.navigation.get()).toMatchObject({ dock: "subagents", subagent: "nested" });
    expect(document.activeElement).toBe(file);
    act(() => screen.getByRole("button", { name: "Read nested" }).focus());
    fireEvent.keyDown(screen.getByRole("complementary", { name: "Subagents" }), {
      key: "Escape",
    });
    expect(testClient.navigation.get().subagent).toBe("child");
    expect(document.activeElement).toBe(nested);
    fireEvent.keyDown(screen.getByRole("complementary", { name: "Subagents" }), {
      key: "Escape",
    });
    expect(document.activeElement).toBe(source);
    expect(testClient.navigation.get()).toMatchObject({ dock: "subagents", subagent: "child" });
    expect(screen.queryByRole("button", { name: "Back to conversation" })).toBeNull();
  });

  it("retains a filtered catalog and returns to its search control after inspection", () => {
    showWorkspaceDock(testClient.workspace.access);
    testClient.render(<ChatPanel onSend={() => true} />);
    const search = screen.getByRole("combobox", { name: "Find a panel…" });
    act(() => search.focus());
    fireEvent.change(search, { target: { value: "Files" } });
    fireEvent.click(screen.getByRole("option", { name: "Files" }));
    const dock = screen.getByRole("complementary", { name: "Files" });
    expect(document.activeElement).toBe(dock);
    fireEvent.click(screen.getByRole("button", { name: "Back to panel list" }));

    expect(testClient.navigation.get().dock).toBe("catalog");
    expect(screen.getByRole("combobox", { name: "Find a panel…" })).toBe(search);
    expect((search as HTMLInputElement).value).toBe("Files");
    expect(document.activeElement).toBe(search);
    expect(screen.queryByRole("option", { name: "Subagents" })).toBeNull();
    fireEvent.keyDown(search, { key: "Escape" });
    expect((search as HTMLInputElement).value).toBe("");
    expect(document.activeElement).toBe(search);
    expect(testClient.navigation.get().dock).toBe("catalog");
    expect(screen.getByRole("option", { name: "Subagents" })).toBeTruthy();
  });

  it("enters a picker selection after its popup retires and returns to the previous panel", async () => {
    openWorkspaceViewInDock(testClient.workspace.access, "subagents");
    testClient.render(<ChatPanel onSend={() => true} />);
    fireEvent.click(screen.getByRole("button", { name: "Evidence 0" }));
    const browse = screen.getByRole("button", { name: "Browse panels" });
    fireEvent.click(browse);
    fireEvent.click(await screen.findByRole("option", { name: "Files" }));
    await vi.waitFor(() => {
      const dock = screen.getByRole("complementary", { name: "Files" });
      expect(document.activeElement).toBe(dock);
      expect(screen.queryByRole("combobox", { name: "Find a panel…" })).toBeNull();
    });
    fireEvent.click(screen.getByRole("button", { name: "Back to Subagents" }));

    expect(testClient.navigation.get().dock).toBe("subagents");
    expect(document.activeElement).toBe(browse);
    expect(screen.getByRole("button", { name: "Evidence 1" })).toBeTruthy();
  });

  it("moves a full view beside the conversation without creating a self-inspection", () => {
    openWorkspaceView(testClient.workspace.access, "file");
    testClient.render(<ChatPanel onSend={() => true} />);
    const viewport = screen.getByRole("region", { name: "Files" });
    fireEvent.click(screen.getByRole("button", { name: "Open in the context dock" }));
    expect(testClient.navigation.get().view).toBeNull();
    const dock = screen.getByRole("complementary", { name: "Files" });
    expect(dock.contains(document.activeElement)).toBe(true);
    expect(screen.getByRole("tabpanel", { name: "Files" })).toBe(viewport);
    expect(screen.queryByRole("button", { name: "Back to Files" })).toBeNull();
  });

  it("enters explicitly opened material and returns to its source without closing the Dock", () => {
    testClient.render(<ChatPanel onSend={() => true} />);
    const source = screen.getByRole("button", { name: "src/main.ts:4" });
    act(() => source.focus());
    fireEvent.click(source);
    const dock = screen.getByRole("complementary", { name: "Files" });
    expect(dock.contains(document.activeElement)).toBe(true);
    expect(screen.getByRole("button", { name: "Back to conversation" })).toBeTruthy();

    fireEvent.keyDown(dock, { key: "Escape" });
    expect(document.activeElement).toBe(source);
    expect(testClient.navigation.get().dock).toBe("file");
    expect(screen.queryByRole("button", { name: "Back to conversation" })).toBeNull();
  });

  it("returns to the original child reader with its component state intact", () => {
    openWorkspaceViewInDock(testClient.workspace.access, "subagents");
    testClient.render(<ChatPanel onSend={() => true} />);
    fireEvent.click(screen.getByRole("button", { name: "Evidence 0" }));
    const source = screen.getByRole("button", { name: "src/child.ts:8" });
    act(() => source.focus());
    fireEvent.click(source);
    expect(testClient.navigation.get().dock).toBe("file");
    expect(document.activeElement).not.toBe(source);
    fireEvent.click(screen.getByRole("button", { name: "Back to Subagents" }));

    expect(testClient.navigation.get().dock).toBe("subagents");
    expect(document.activeElement).toBe(source);
    expect(screen.getByRole("button", { name: "Evidence 1" })).toBeTruthy();
  });

  it("keeps focus where the user moved it before entry and during a background update", () => {
    testClient.render(<ChatPanel onSend={() => true} />);
    const source = screen.getByRole("button", { name: "src/main.ts:4" });
    const other = screen.getByRole("button", { name: "Other control" });
    act(() => {
      source.focus();
      openWorkspaceFile(testClient.workspace.access, "src/main.ts", 4, source);
      other.focus();
    });
    expect(document.activeElement).toBe(other);
    act(() => openWorkspaceFile(testClient.workspace.access, "src/updated.ts"));
    expect(document.activeElement).toBe(other);
  });

  it("retires the source element when the Session changes", () => {
    const view = testClient.render(<ChatPanel onSend={() => true} />);
    fireEvent.click(screen.getByRole("button", { name: "src/main.ts:4" }));
    const dock = screen.getByRole("complementary", { name: "Files" });
    expect(within(dock).getByRole("button", { name: "Back to conversation" })).toBeTruthy();
    act(() => {
      model.sessionId = "ses_second";
      activateWorkspaceSessionScope(testClient.workspace.access, model.sessionId);
    });
    view.rerender(<ChatPanel onSend={() => true} />);
    expect(screen.queryByRole("button", { name: "Back to conversation" })).toBeNull();
  });
});

describe("table preview file references", () => {
  function conversation() {
    return screen.getByRole("region", { name: "Conversation" });
  }

  it("ends the preview, opens the exact line in the Dock and returns to the source table", async () => {
    const viewer = testClient.renderHook(useWorkspaceFileViewer);
    testClient.render(<ChatPanel onSend={() => true} />);
    const { expand, dialog } = await openPreview(conversation());
    const reference = within(dialog).getByRole("button", { name: "src/stream.ts:2" });
    act(() => reference.focus());
    fireEvent.click(reference, { detail: 1 });
    await settle();

    expect(screen.queryByRole("dialog", { name: "Table preview" })).toBeNull();
    expect(viewer.result.current).toEqual({ path: "src/stream.ts", line: 2 });
    expect(testClient.navigation.get().dock).toBe("file");
    const dock = screen.getByRole("complementary", { name: "Files" });
    expect(document.activeElement).toBe(dock);

    fireEvent.keyDown(dock, { key: "Escape" });
    expect(document.activeElement).toBe(expand);
  });

  it("returns through a task table preview to the same task and then the conversation", async () => {
    const viewer = testClient.renderHook(useWorkspaceFileViewer);
    testClient.render(<ChatPanel onSend={() => true} />);
    const source = screen.getByRole("button", { name: "Inspect task" });
    act(() => source.focus());
    fireEvent.click(source);
    const task = screen.getByRole("complementary", { name: "Subagents" });
    fireEvent.click(within(task).getByRole("button", { name: "Evidence 0" }));
    const { expand, dialog } = await openPreview(task);
    const reference = within(dialog).getByRole("button", { name: "README.md:4" });
    act(() => reference.focus());
    fireEvent.keyDown(reference, { key: "Enter" });
    await settle();

    expect(screen.queryByRole("dialog", { name: "Table preview" })).toBeNull();
    expect(viewer.result.current).toEqual({ path: "README.md", line: 4 });
    expect(document.activeElement).toBe(screen.getByRole("complementary", { name: "Files" }));

    fireEvent.click(screen.getByRole("button", { name: "Back to task" }));
    expect(testClient.navigation.get()).toMatchObject({ dock: "subagents", subagent: "child" });
    expect(document.activeElement).toBe(expand);
    expect(screen.getByRole("button", { name: "Evidence 1" })).toBeTruthy();

    fireEvent.keyDown(screen.getByRole("complementary", { name: "Subagents" }), {
      key: "Escape",
    });
    expect(document.activeElement).toBe(source);
  });

  it("returns a main-only table preview inspection to the same main source", async () => {
    model.views.push({
      id: "main-table",
      title: "Table material",
      component: () => <AnswerTurn text={TABLE} />,
    });
    openWorkspaceView(testClient.workspace.access, "main-table");
    testClient.render(<ChatPanel onSend={() => true} />);
    const { expand, dialog } = await openPreview(
      screen.getByRole("region", { name: "Table material" }),
    );
    fireEvent.click(within(dialog).getByRole("button", { name: "app/ui/resizer.ts:6" }));
    await settle();
    expect(testClient.navigation.get()).toMatchObject({ view: null, dock: "file" });

    fireEvent.keyDown(screen.getByRole("complementary", { name: "Files" }), { key: "Escape" });
    expect(testClient.navigation.get().view).toBe("main-table");
    expect(document.activeElement).toBe(expand);
  });

  it("keeps the inline table reference as its own origin", async () => {
    testClient.render(<ChatPanel onSend={() => true} />);
    const reference = await within(conversation()).findByRole("button", {
      name: "src/stream.ts:2",
    });
    act(() => reference.focus());
    fireEvent.click(reference, { detail: 1 });
    expect(testClient.navigation.get().dock).toBe("file");

    fireEvent.keyDown(screen.getByRole("complementary", { name: "Files" }), { key: "Escape" });
    expect(document.activeElement).toBe(reference);
  });

  it("keeps a selected reference inert without blocking keyboard activation", async () => {
    const viewer = testClient.renderHook(useWorkspaceFileViewer);
    testClient.render(<ChatPanel onSend={() => true} />);
    const { dialog } = await openPreview(conversation());
    const reference = within(dialog).getByRole("button", { name: "docs/selected.md:9" });
    const range = document.createRange();
    range.selectNodeContents(reference.closest("td")!);
    const selection = document.getSelection()!;
    selection.removeAllRanges();
    selection.addRange(range);
    try {
      fireEvent.click(reference, { detail: 1 });
      expect(screen.getByRole("dialog", { name: "Table preview" })).toBe(dialog);
      expect(viewer.result.current?.path).not.toBe("docs/selected.md");
      expect(testClient.navigation.get().dock).toBeNull();

      act(() => reference.focus());
      fireEvent.keyDown(reference, { key: "Enter" });
      expect(viewer.result.current).toEqual({ path: "docs/selected.md", line: 9 });
    } finally {
      selection.removeAllRanges();
    }
  });

  it("returns an ordinary close to Expand before and after a navigated preview", async () => {
    testClient.render(<ChatPanel onSend={() => true} />);
    let { expand, dialog } = await openPreview(conversation());
    fireEvent.keyDown(dialog, { key: "Escape" });
    await settle();
    expect(screen.queryByRole("dialog", { name: "Table preview" })).toBeNull();
    expect(testClient.navigation.get().dock).toBeNull();
    expect(document.activeElement).toBe(expand);

    ({ dialog } = await openPreview(conversation()));
    fireEvent.click(within(dialog).getByRole("button", { name: "src/stream.ts:2" }));
    await settle();
    fireEvent.keyDown(screen.getByRole("complementary", { name: "Files" }), { key: "Escape" });
    expect(document.activeElement).toBe(expand);

    ({ expand, dialog } = await openPreview(conversation()));
    fireEvent.click(within(dialog).getByRole("button", { name: "Close table preview" }));
    await settle();
    expect(screen.queryByRole("dialog", { name: "Table preview" })).toBeNull();
    expect(document.activeElement).toBe(expand);
  });
});

describe("material inspection without room for the Dock", () => {
  it("presents an inspected file full width at its line and keeps it across resizes", async () => {
    const resize = observeRowWidth(700);
    const viewer = testClient.renderHook(useWorkspaceFileViewer);
    const ratio = testClient.renderHook(useDockWidth);
    openWorkspaceViewInDock(testClient.workspace.access, "file");
    testClient.render(<ChatPanel onSend={() => true} />);
    expect(screen.getByRole("region", { name: "Conversation" })).toBeTruthy();
    expect(windowHeader()).toBeNull();

    const source = screen.getByRole("button", { name: "src/main.ts:4" });
    act(() => source.focus());
    fireEvent.click(source);
    expect(viewer.result.current).toEqual({ path: "src/main.ts", line: 4 });
    expect(document.activeElement).toBe(screen.getByRole("region", { name: "Files" }));
    expect(screen.queryByRole("region", { name: "Conversation" })).toBeNull();
    const header = within(windowHeader()!);
    expect(header.getByRole("button", { name: "Back to conversation" })).toBeTruthy();
    expect(header.queryByRole("button", { name: "Close" })).toBeNull();
    expect(header.queryByRole("button", { name: "Open in the context dock" })).toBeNull();

    const location = testClient.navigation.get();
    const savedRatio = ratio.result.current.width;
    const controls = screen.getByRole("button", { name: "File controls" });
    await resize(1200);
    expect(windowHeader()).toBeNull();
    expect(screen.getByRole("region", { name: "Conversation" })).toBeTruthy();
    const dock = screen.getByRole("complementary", { name: "Files" });
    expect(within(dock).getByRole("button", { name: "Back to conversation" })).toBeTruthy();
    act(() => dock.focus());
    await resize(700);
    expect(document.activeElement).toBe(screen.getByRole("region", { name: "Files" }));
    expect(
      within(windowHeader()!).getByRole("button", { name: "Back to conversation" }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "File controls" })).toBe(controls);
    expect(testClient.navigation.get()).toEqual(location);
    expect(ratio.result.current.width).toBe(savedRatio);

    fireEvent.keyDown(controls, { key: "Escape" });
    expect(document.activeElement).toBe(source);
    expect(windowHeader()).toBeNull();
    expect(testClient.navigation.get()).toMatchObject({ view: null, dock: "file" });
  });

  it("returns through a full-width task, table preview and file to the conversation", async () => {
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(700);
    const viewer = testClient.renderHook(useWorkspaceFileViewer);
    testClient.render(<ChatPanel onSend={() => true} />);
    const source = screen.getByRole("button", { name: "Inspect task" });
    act(() => source.focus());
    fireEvent.click(source);
    expect(document.activeElement).toBe(screen.getByRole("region", { name: "Subagents" }));
    expect(
      within(windowHeader()!).getByRole("button", { name: "Back to conversation" }),
    ).toBeTruthy();
    const nested = screen.getByRole("button", { name: "Inspect nested task" });
    act(() => nested.focus());
    fireEvent.click(nested);
    expect(testClient.navigation.get().subagent).toBe("nested");

    const { expand, dialog } = await openPreview(screen.getByRole("region", { name: "Subagents" }));
    const reference = within(dialog).getByRole("button", { name: "README.md:4" });
    act(() => reference.focus());
    fireEvent.keyDown(reference, { key: "Enter" });
    await settle();
    expect(viewer.result.current).toEqual({ path: "README.md", line: 4 });
    expect(document.activeElement).toBe(screen.getByRole("region", { name: "Files" }));

    fireEvent.click(within(windowHeader()!).getByRole("button", { name: "Back to task" }));
    expect(testClient.navigation.get()).toMatchObject({
      view: null,
      dock: "subagents",
      subagent: "nested",
    });
    expect(document.activeElement).toBe(expand);
    fireEvent.keyDown(within(windowHeader()!).getByRole("button", { name: "Back to task" }), {
      key: "Escape",
    });
    expect(testClient.navigation.get()).toMatchObject({ view: null, subagent: "child" });
    expect(document.activeElement).toBe(nested);
    fireEvent.keyDown(screen.getByRole("button", { name: "Read child" }), { key: "Escape" });
    expect(document.activeElement).toBe(source);
    expect(windowHeader()).toBeNull();
  });

  it.each([
    { width: 700, exit: "header" },
    { width: 1200, exit: "body" },
  ] as const)(
    "returns an inspected task presented as main through a file to the conversation at $width",
    ({ width, exit }) => {
      vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(width);
      testClient.render(<ChatPanel onSend={() => true} />);
      const source = screen.getByRole("button", { name: "Inspect task" });
      act(() => source.focus());
      fireEvent.click(source);
      act(() => openWorkspaceView(testClient.workspace.access, "subagents"));
      expect(testClient.navigation.get()).toMatchObject({ view: "subagents", subagent: "child" });
      let header = within(windowHeader()!);
      expect(header.getByRole("button", { name: "Back to conversation" })).toBeTruthy();
      expect(header.queryByRole("button", { name: "Close" })).toBeNull();

      fireEvent.click(screen.getByRole("button", { name: "Evidence 0" }));
      const file = screen.getByRole("button", { name: "src/child.ts:8" });
      act(() => file.focus());
      fireEvent.click(file);
      expect(testClient.navigation.get()).toMatchObject({ view: null, dock: "file" });
      fireEvent.click(screen.getByRole("button", { name: "Back to task" }));

      expect(testClient.navigation.get()).toMatchObject({ view: "subagents", subagent: "child" });
      expect(document.activeElement).toBe(file);
      expect(screen.getByRole("button", { name: "Evidence 1" })).toBeTruthy();
      header = within(windowHeader()!);
      const back = header.getByRole("button", { name: "Back to conversation" });
      expect(header.queryByRole("button", { name: "Close" })).toBeNull();

      fireEvent.keyDown(
        exit === "header" ? back : screen.getByRole("button", { name: "Read child" }),
        { key: "Escape" },
      );
      expect(document.activeElement).toBe(source);
      expect(testClient.navigation.get().view).toBeNull();
      expect(windowHeader()).toBeNull();
      expect(screen.queryByRole("button", { name: "Back to conversation" })).toBeNull();
    },
  );

  it.each(["header", "body"])(
    "returns a nested task opened in explicit main through a file using %s",
    (exit) => {
      vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(700);
      openWorkspaceViewInDock(testClient.workspace.access, "subagents");
      openWorkspaceSubagentRun(testClient.workspace.access, "child");
      testClient.render(<ChatPanel onSend={() => true} />);
      const draft = screen.getByLabelText<HTMLTextAreaElement>("Composer draft");
      fireEvent.click(screen.getByRole("button", { name: /Open material full width/ }));
      expect(testClient.navigation.get()).toMatchObject({ view: "subagents", subagent: "child" });
      expect(within(windowHeader()!).getByRole("button", { name: "Close" })).toBeTruthy();
      fireEvent.click(screen.getByRole("button", { name: "Evidence 0" }));

      const nested = screen.getByRole("button", { name: "Inspect nested task" });
      act(() => nested.focus());
      fireEvent.click(nested);
      expect(testClient.navigation.get()).toMatchObject({ view: "subagents", subagent: "nested" });
      expect(screen.getByText("Selected task: nested")).toBeTruthy();
      expect(within(windowHeader()!).getByRole("button", { name: "Back to task" })).toBeTruthy();
      expect(within(windowHeader()!).queryByRole("button", { name: "Close" })).toBeNull();

      const file = screen.getByRole("button", { name: "src/child.ts:8" });
      act(() => file.focus());
      fireEvent.click(file);
      expect(testClient.navigation.get()).toMatchObject({ view: null, dock: "file" });
      fireEvent.click(within(windowHeader()!).getByRole("button", { name: "Back to task" }));
      expect(testClient.navigation.get()).toMatchObject({ view: "subagents", subagent: "nested" });
      expect(document.activeElement).toBe(file);

      const returnControl =
        exit === "header"
          ? within(windowHeader()!).getByRole("button", { name: "Back to task" })
          : screen.getByRole("button", { name: "Read nested" });
      act(() => returnControl.focus());
      if (exit === "header") fireEvent.click(returnControl);
      else fireEvent.keyDown(returnControl, { key: "Escape" });
      expect(testClient.navigation.get()).toMatchObject({ view: "subagents", subagent: "child" });
      expect(document.activeElement).toBe(nested);
      expect(screen.getByText("Selected task: child")).toBeTruthy();
      expect(screen.getByRole("button", { name: "Evidence 1" })).toBeTruthy();
      const header = within(windowHeader()!);
      expect(header.getByRole("button", { name: "Close" })).toBeTruthy();
      expect(header.queryByRole("button", { name: "Back to task" })).toBeNull();
      expect(screen.getByLabelText("Composer draft")).toBe(draft);
      expect(draft.value).toBe(INITIAL_DRAFT);
      fireEvent.click(header.getByRole("button", { name: "Close" }));
      const conversation = screen.getByRole("region", { name: "Conversation" });
      expect(within(conversation).getByLabelText("Composer draft")).toBe(draft);
      act(() => draft.focus());
      fireEvent.change(draft, { target: { value: `${INITIAL_DRAFT} Continued.` } });
      expect(captureComposerSubmission(testClient.composer.access).value).toBe(
        `${INITIAL_DRAFT} Continued.`,
      );
      expect(document.activeElement).toBe(draft);
    },
  );

  it("leaves outside, composing and local Escape to their owners before returning", () => {
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(700);
    model.views[0] = { ...model.views[0]!, component: FilteredMaterial };
    testClient.render(
      <>
        <button>Outside control</button>
        <ChatPanel onSend={() => true} />
      </>,
    );
    const outside = screen.getByRole("button", { name: "Outside control" });
    const source = screen.getByRole("button", { name: "src/main.ts:4" });
    act(() => {
      source.focus();
      openWorkspaceFile(testClient.workspace.access, "src/main.ts", 4, source);
      outside.focus();
    });
    expect(document.activeElement).toBe(outside);
    fireEvent.keyDown(outside, { key: "Escape" });
    const back = within(windowHeader()!).getByRole("button", { name: "Back to conversation" });

    const filter = screen.getByRole<HTMLInputElement>("searchbox", { name: "Filter material" });
    act(() => filter.focus());
    fireEvent.keyDown(filter, { key: "Escape", isComposing: true });
    expect(filter.value).toBe("src/main");
    fireEvent.keyDown(filter, { key: "Escape" });
    expect(filter.value).toBe("");
    expect(back.isConnected).toBe(true);
    fireEvent.keyDown(filter, { key: "Escape" });
    expect(back.isConnected).toBe(false);
    expect(document.activeElement).toBe(source);
  });

  it("restores an ordinary Dock source full width when the row narrowed during inspection", async () => {
    const resize = observeRowWidth(1200);
    openWorkspaceViewInDock(testClient.workspace.access, "subagents");
    testClient.render(<ChatPanel onSend={() => true} />);
    fireEvent.click(screen.getByRole("button", { name: "Evidence 0" }));
    const source = screen.getByRole("button", { name: "src/child.ts:8" });
    act(() => source.focus());
    fireEvent.click(source);
    await resize(700);

    fireEvent.click(within(windowHeader()!).getByRole("button", { name: "Back to Subagents" }));
    expect(screen.queryByRole("region", { name: "Conversation" })).toBeNull();
    expect(screen.getByRole("button", { name: "Evidence 1" })).toBeTruthy();
    expect(document.activeElement).toBe(source);
    const header = within(windowHeader()!);
    expect(header.getByRole("button", { name: "Close" })).toBeTruthy();
    expect(header.queryByRole("button", { name: "Open in the context dock" })).toBeNull();
  });

  it("restores a filtered catalog source full width with its search control", async () => {
    const resize = observeRowWidth(1200);
    showWorkspaceDock(testClient.workspace.access);
    testClient.render(<ChatPanel onSend={() => true} />);
    const search = screen.getByRole<HTMLInputElement>("combobox", { name: "Find a panel…" });
    act(() => search.focus());
    fireEvent.change(search, { target: { value: "Files" } });
    fireEvent.click(screen.getByRole("option", { name: "Files" }));
    await resize(700);

    fireEvent.click(within(windowHeader()!).getByRole("button", { name: "Back to panel list" }));
    expect(screen.queryByRole("region", { name: "Conversation" })).toBeNull();
    expect(screen.getByRole("combobox", { name: "Find a panel…" })).toBe(search);
    expect(search.value).toBe("Files");
    expect(document.activeElement).toBe(search);
    expect(screen.queryByText("View unavailable")).toBeNull();

    fireEvent.click(within(windowHeader()!).getByRole("button", { name: "Close" }));
    fireEvent.click(screen.getByRole("button", { name: /Open material full width/ }));
    expect(screen.getByRole("combobox", { name: "Find a panel…" })).toBe(search);
    expect(search.value).toBe("Files");
  });
});

describe("preview history lifetime", () => {
  it("preserves later outside focus while retiring a conversation preview", async () => {
    openWorkspaceView(testClient.workspace.access, "file");
    testClient.render(
      <>
        <button>Later navigation</button>
        <ChatPanel onSend={() => true} />
      </>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    const later = screen.getByRole("button", { name: "Later navigation" });
    const trigger = screen.getByRole("button", { name: "Expand table" });
    act(() => trigger.focus());
    fireEvent.click(trigger);
    await settle();

    act(() => {
      testClient.navigation.back();
      later.focus();
    });
    await settle();

    expect(testClient.navigation.get().view).toBe("file");
    expect(screen.queryByRole("dialog", { name: "Table preview" })).toBeNull();
    expect(document.activeElement).toBe(later);
  });

  it("retires a retained main task preview when history presents another material", async () => {
    openWorkspaceViewInDock(testClient.workspace.access, "subagents");
    openWorkspaceView(testClient.workspace.access, "file");
    openWorkspaceView(testClient.workspace.access, "subagents");
    testClient.render(<ChatPanel onSend={() => true} />);
    const source = screen.getByRole("button", { name: "Evidence 0" });
    const trigger = screen.getByRole("button", { name: "Expand table" });
    act(() => trigger.focus());
    fireEvent.click(trigger);
    await settle();

    act(() => testClient.navigation.back());
    await settle();

    expect(screen.queryByRole("dialog", { name: "Table preview" })).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("region", { name: "Files" }));
    act(() => testClient.navigation.forward());
    await settle();
    expect(screen.queryByRole("dialog", { name: "Table preview" })).toBeNull();
    expect(screen.getByRole("button", { name: "Evidence 0" })).toBe(source);
    expect(screen.getByRole("button", { name: "Expand table" })).toBe(trigger);
    fireEvent.click(trigger);
    expect(screen.getByRole("dialog", { name: "Table preview" })).toBeTruthy();
  });

  it("retires a table preview when history hides its conversation", async () => {
    openWorkspaceView(testClient.workspace.access, "file");
    testClient.render(<ChatPanel onSend={() => true} />);
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    const draft = screen.getByLabelText<HTMLTextAreaElement>("Composer draft");
    const trigger = screen.getByRole("button", { name: "Expand table" });
    act(() => trigger.focus());
    fireEvent.click(trigger);
    await settle();
    expect(screen.getByRole("dialog", { name: "Table preview" })).toBeTruthy();
    act(() => testClient.navigation.back());
    await settle();
    expect(testClient.navigation.get().view).toBe("file");
    expect(screen.queryByRole("dialog", { name: "Table preview" })).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("region", { name: "Files" }));
    act(() => testClient.navigation.forward());
    await settle();
    expect(screen.queryByRole("dialog", { name: "Table preview" })).toBeNull();
    expect(screen.getByLabelText("Composer draft")).toBe(draft);
    expect(draft.value).toBe("Keep this draft — 保留");
    fireEvent.click(trigger);
    expect(screen.getByRole("dialog", { name: "Table preview" })).toBeTruthy();
  });
});

describe("shared material popup visibility", () => {
  it.each([
    ["select", "collapse"],
    ["menu", "collapse"],
    ["select", "retreat"],
    ["menu", "retreat"],
  ] as const)(
    "retires a shared %s on %s without replacing retained material or its draft",
    async (kind, transition) => {
      const resize = observeRowWidth(1200);
      const select = vi.fn();
      const command = vi.fn();
      function SharedMaterial() {
        const [value, setValue] = useState("working");
        return (
          <>
            <ChildReader />
            <Select
              label="Material comparison"
              value={value}
              onValueChange={(next) => {
                select(next);
                setValue(next);
              }}
              options={[
                { value: "working", label: "Working changes" },
                { value: "branch", label: "Branch changes" },
              ]}
            />
            <ActionMenu
              label="Material record actions"
              items={[{ label: "Inspect record", onSelect: command }]}
            />
          </>
        );
      }
      model.views[1] = { ...model.views[1]!, component: SharedMaterial };
      openWorkspaceViewInDock(testClient.workspace.access, "subagents");
      testClient.render(<ChatPanel onSend={() => true} />);
      fireEvent.click(screen.getByRole("button", { name: "Evidence 0" }));
      const child = screen.getByRole("button", { name: "Evidence 1" });
      const draft = screen.getByLabelText<HTMLTextAreaElement>("Composer draft");
      draft.setSelectionRange(5, 5);
      const trigger = screen.getByRole(kind === "select" ? "combobox" : "button", {
        name: kind === "select" ? "Material comparison" : "Material record actions",
      });
      fireEvent.click(trigger);
      const popup = await screen.findByRole(kind === "select" ? "listbox" : "menu");
      const item = within(popup).getByRole(kind === "select" ? "option" : "menuitem", {
        name: kind === "select" ? "Working changes" : "Inspect record",
      });
      act(() => item.focus());

      if (transition === "collapse") act(() => toggleWorkspaceDock(testClient.workspace.access));
      else await resize(700);

      await vi.waitFor(() => {
        expect(screen.queryByRole(kind === "select" ? "listbox" : "menu")).toBeNull();
        expect(document.activeElement).toBe(screen.getByRole("region", { name: "Conversation" }));
      });
      expect(child.isConnected).toBe(true);
      expect(select).not.toHaveBeenCalled();
      expect(command).not.toHaveBeenCalled();

      if (transition === "collapse") act(() => toggleWorkspaceDock(testClient.workspace.access));
      else await resize(1200);
      await settle();

      expect(screen.queryByRole(kind === "select" ? "listbox" : "menu")).toBeNull();
      expect(screen.getByRole("button", { name: "Evidence 1" })).toBe(child);
      expect(
        screen.getByRole(kind === "select" ? "combobox" : "button", {
          name: kind === "select" ? "Material comparison" : "Material record actions",
        }),
      ).toBe(trigger);
      expect(screen.getByRole("combobox", { name: "Material comparison" }).textContent).toContain(
        "Working changes",
      );
      expect(screen.getByLabelText("Composer draft")).toBe(draft);
      expect(draft.value).toBe("Keep this draft — 保留");
      expect(draft.selectionEnd).toBe(5);
    },
  );
});

describe("bounded material tasks", () => {
  it.each([
    ["select", "collapse"],
    ["menu", "collapse"],
    ["select", "retreat"],
    ["menu", "retreat"],
  ] as const)(
    "keeps a task's %s active through %s and returns to the visible workspace on dismissal",
    async (kind, transition) => {
      const resize = observeRowWidth(1200);
      const change = vi.fn();
      const command = vi.fn();
      function MaterialTask() {
        const [open, setOpen] = useState(false);
        const [title, setTitle] = useState("Retained bounded draft 保留");
        const opener = useRef<HTMLButtonElement>(null);
        return (
          <>
            <ChildReader />
            <button ref={opener} onClick={() => setOpen(true)}>
              Edit material task
            </button>
            <ModalDialog
              open={open}
              onOpenChange={setOpen}
              title="Bounded material task"
              finalFocus={opener}
            >
              <input
                aria-label="Task draft"
                value={title}
                onChange={(event) => setTitle(event.target.value)}
              />
              <Select
                label="Task model"
                value="default"
                onValueChange={change}
                options={[
                  { value: "default", label: "Runtime default" },
                  { value: "chosen", label: "Chosen model" },
                ]}
              />
              <ActionMenu
                label="Task actions"
                items={[{ label: "Inspect task", onSelect: command }]}
              />
            </ModalDialog>
          </>
        );
      }
      model.views[1] = { ...model.views[1]!, component: MaterialTask };
      openWorkspaceViewInDock(testClient.workspace.access, "subagents");
      testClient.render(<ChatPanel onSend={() => true} />);
      fireEvent.click(screen.getByRole("button", { name: "Evidence 0" }));
      const reader = screen.getByRole("button", { name: "Evidence 1" });
      const composer = screen.getByLabelText<HTMLTextAreaElement>("Composer draft");
      composer.setSelectionRange(5, 5);
      const opener = screen.getByRole("button", { name: "Edit material task" });
      fireEvent.click(opener);
      const dialog = await screen.findByRole("dialog", { name: "Bounded material task" });
      const field = within(dialog).getByRole<HTMLInputElement>("textbox", { name: "Task draft" });
      fireEvent.change(field, { target: { value: "Updated bounded draft 保留" } });
      const trigger = within(dialog).getByRole(kind === "select" ? "combobox" : "button", {
        name: kind === "select" ? "Task model" : "Task actions",
      });
      fireEvent.click(trigger);
      const popupRole = kind === "select" ? "listbox" : "menu";
      const popup = await screen.findByRole(popupRole);
      const item = within(popup).getByRole(kind === "select" ? "option" : "menuitem", {
        name: kind === "select" ? "Runtime default" : "Inspect task",
      });
      act(() => item.focus());

      if (transition === "collapse") act(() => toggleWorkspaceDock(testClient.workspace.access));
      else await resize(700);
      await settle();

      expect(screen.getByRole("dialog", { name: "Bounded material task" })).toBe(dialog);
      vi.spyOn(opener, "getClientRects").mockReturnValue([] as unknown as DOMRectList);
      expect(screen.getByRole(popupRole)).toBe(popup);
      expect(popup.contains(document.activeElement)).toBe(true);
      expect(field.value).toBe("Updated bounded draft 保留");
      expect(change).not.toHaveBeenCalled();
      expect(command).not.toHaveBeenCalled();

      await act(async () => {
        fireEvent.keyDown(document.activeElement!, { key: "Escape" });
      });
      await vi.waitFor(() => expect(screen.queryByRole(popupRole)).toBeNull());
      await vi.waitFor(() => expect(document.activeElement).toBe(trigger));
      expect(screen.getByRole("dialog", { name: "Bounded material task" })).toBe(dialog);
      await act(async () => {
        fireEvent.keyDown(trigger, { key: "Escape" });
      });
      await vi.waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
      const conversation = screen.getByRole("region", { name: "Conversation" });
      await vi.waitFor(() =>
        expect(document.activeElement).toBe(
          within(conversation).getByRole("button", { name: "More actions for New session" }),
        ),
      );

      if (transition === "collapse") act(() => toggleWorkspaceDock(testClient.workspace.access));
      else await resize(1200);
      expect(screen.getByRole("button", { name: "Edit material task" })).toBe(opener);
      expect(screen.getByRole("button", { name: "Evidence 1" })).toBe(reader);
      expect(screen.queryByRole("dialog")).toBeNull();
      expect(screen.getByLabelText("Composer draft")).toBe(composer);
      expect(composer.value).toBe("Keep this draft — 保留");
      expect(composer.selectionEnd).toBe(5);
    },
  );
});

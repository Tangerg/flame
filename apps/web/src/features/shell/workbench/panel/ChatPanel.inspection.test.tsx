import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  within,
} from "@testing-library/react";
import { createRef, useEffect, useRef, useState } from "react";
import { ActionMenu, ModalDialog, SearchField, Select } from "@flame/ui";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type WorkspaceViewSpec } from "@/features/workspace/public/views";
import { navigator } from "@/lib/navigation";
import { FileRefLink } from "@/features/workspace/public/FileRefLink";
import { MessageTurn } from "@/features/chat/public/message/rendering";
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
import { ContextMenu, Popover } from "@/ui/index";
const TABLE = "| File | Note |\n|---|---|\n| app/ui/resizer.ts:6 | README.md:4 |";

function DraftInput() {
  const input = useRef<HTMLTextAreaElement>(null);
  useEffect(() => registerComposerFocusTarget(input.current), []);
  return <textarea ref={input} aria-label="Composer draft" defaultValue="Keep this draft — 保留" />;
}

function AnswerTurn({ text }: { text: string }) {
  return (
    <MessageTurn
      row={{
        message: {
          id: "answer",
          runId: "run-answer",
          role: "assistant",
          blocks: [{ kind: "text", itemId: "answer-text", text, status: "complete" }],
        },
        runOwner: { kind: "owned", runId: "run-answer", status: "finished" },
        facts: { toolCalls: {}, delegatedRuns: {}, awaiting: new Map<string, string>() },
      }}
      ctx={{ expandedIds: new Set(), onToggleExpand: () => {}, textReveal: "instant" }}
      sessionId="ses_first"
      isLast
      isRunning={false}
    />
  );
}

const model = vi.hoisted(() => ({ sessionId: "ses_first", views: [] as WorkspaceViewSpec[] }));

vi.mock("@/features/agent/public/session", () => ({
  useActiveSession: () => null,
  useActiveSessionId: () => model.sessionId,
  useAgentSessions: () => ({ isLoading: false }),
}));

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
        <AnswerTurn text={"| Path |\n|---|\n| src/stream.ts:2 |\n| docs/selected.md:9 |"} />
        <button onClick={(event) => openWorkspaceSubagentRun("child", event.currentTarget)}>
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
      <button onClick={(event) => openWorkspaceSubagentRun("nested", event.currentTarget)}>
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
  activateWorkspaceSessionScope(model.sessionId);
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

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("hands focus from a removed full-width trigger to the material and back to the conversation", () => {
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(700);
  openWorkspaceViewInDock("file");
  render(<ChatPanel onSend={() => true} />);
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
  openWorkspaceViewInDock("file");
  openWorkspaceView("file");
  render(<ChatPanel onSend={() => true} />);
  const close = screen.getByRole("button", { name: "Close" });
  act(() => close.focus());
  fireEvent.click(close);
  expect(document.activeElement).toBe(screen.getByRole("complementary", { name: "Files" }));
});

it("preserves outside focus when a main view is presented", () => {
  render(
    <>
      <button>Outside control</button>
      <ChatPanel onSend={() => true} />
    </>,
  );
  const outside = screen.getByRole("button", { name: "Outside control" });
  act(() => outside.focus());
  act(() => openWorkspaceView("file"));
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
    const view = render(
      <>
        <button>Later control</button>
        <ChatPanel ref={panel} onSend={() => true} />
      </>,
    );
    const draft = screen.getByLabelText<HTMLTextAreaElement>("Composer draft");
    return { panel, view, draft };
  }

  function settings() {
    openWorkspaceViewInDock("file");
    openWorkspaceView("settings");
    const result = mount();
    act(() => screen.getByRole("button", { name: "Settings controls" }).focus());
    return result;
  }

  it("reveals the existing draft and preserves the wide Dock and one-step history", () => {
    const { panel, draft } = settings();
    const previous = navigator().get();
    draft.setSelectionRange(5, 5);

    act(() => panel.current!.focusComposer());

    expect(screen.getByRole("region", { name: "Conversation" })).toBeTruthy();
    expect(screen.getByRole("complementary", { name: "Files" })).toBeTruthy();
    expect(document.activeElement).toBe(draft);
    expect(draft.value).toBe("Keep this draft — 保留");
    expect(draft.selectionEnd).toBe(5);
    expect(navigator().get()).toMatchObject({ view: null, dock: "file" });
    act(() => navigator().back());
    expect(navigator().get()).toEqual(previous);
    expect(screen.getByRole("region", { name: "Settings" })).toBeTruthy();
  });

  it("focuses a visible draft without navigating or hiding the Dock", () => {
    openWorkspaceViewInDock("file");
    const { panel, draft } = mount();
    const previous = navigator().get();
    act(() => screen.getByRole("button", { name: "Other control" }).focus());

    act(() => panel.current!.focusComposer());

    expect(document.activeElement).toBe(draft);
    expect(navigator().get()).toBe(previous);
    expect(screen.getByRole("complementary", { name: "Files" })).toBeTruthy();
  });

  it("reveals the conversation from a narrow inspection and retains its source and material", () => {
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(700);
    const { panel, draft } = mount();
    const origin = screen.getByRole("button", { name: "src/main.ts:4" });
    act(() => origin.focus());
    fireEvent.click(origin);
    const controls = screen.getByRole("button", { name: "File controls" });
    const previous = navigator().get();
    act(() => controls.focus());

    act(() => panel.current!.focusComposer());

    expect(navigator().get()).toMatchObject({ view: null, dock: null });
    expect(screen.getByRole("region", { name: "Conversation" })).toBeTruthy();
    expect(document.activeElement).toBe(draft);
    expect(controls.isConnected).toBe(true);
    act(() => navigator().back());
    expect(navigator().get()).toEqual(previous);
    expect(screen.getByRole("button", { name: "File controls" })).toBe(controls);
    expect(screen.getByRole("button", { name: "Back to conversation" })).toBeTruthy();
    expect(draft.value).toBe("Keep this draft — 保留");
  });

  it("manually reopens the inspected material after focusing a narrow conversation", () => {
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(700);
    openWorkspaceViewInDock("subagents");
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

    expect(navigator().get().view).toBeNull();
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
        openWorkspaceView(viewId);
      });

      expect(navigator().get().view).toBe(viewId);
      const close = within(windowHeader()!).getByRole("button", { name: "Close" });
      act(() => close.focus());
      fireEvent.click(close);
      expect(document.activeElement).not.toBe(draft);
      expect(document.activeElement).toBe(screen.getByRole("complementary", { name: "Files" }));
    },
  );

  it("keeps an open modal's focus and underlying presentation", () => {
    const { panel } = settings();
    render(
      <div role="dialog" aria-label="Editing task">
        <button>Modal control</button>
      </div>,
    );
    const control = screen.getByRole("button", { name: "Modal control" });
    act(() => control.focus());
    const previous = navigator().get();

    act(() => panel.current!.focusComposer());

    expect(document.activeElement).toBe(control);
    expect(navigator().get()).toBe(previous);
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
    openWorkspaceViewInDock("file");
    openWorkspaceView("file");
    render(
      <>
        <button>Later navigation focus</button>
        <ChatPanel onSend={() => true} />
      </>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    const draft = screen.getByLabelText<HTMLTextAreaElement>("Composer draft");
    act(() => draft.focus());
    draft.setSelectionRange(4, 4);
    act(() => navigator().back());
    const material = screen.getByRole("region", { name: "Files" });
    expect(document.activeElement).toBe(material);
    return { draft, material, control: screen.getByRole("button", { name: "File controls" }) };
  }

  it.each(["region", "child"])(
    "hands hidden retained material %s focus to the narrow conversation on history forward",
    (origin) => {
      const { draft, material, control } = materialHistory(700);
      if (origin === "child") act(() => control.focus());

      act(() => navigator().forward());

      expect(navigator().get().view).toBeNull();
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

    act(() => navigator().forward());

    expect(screen.getByRole("complementary", { name: "Files" }).contains(control)).toBe(true);
    expect(document.activeElement).toBe(control);
  });

  it("preserves later outside focus when history hides a narrow material", () => {
    materialHistory(700);
    const later = screen.getByRole("button", { name: "Later navigation focus" });

    act(() => {
      navigator().forward();
      later.focus();
    });

    expect(screen.getByRole("region", { name: "Conversation" })).toBeTruthy();
    expect(document.activeElement).toBe(later);
  });

  it("enters a main view when focus belonged to the conversation it hides", () => {
    render(<ChatPanel onSend={() => true} />);
    const control = screen.getByRole("button", { name: "Other control" });
    act(() => control.focus());
    act(() => openWorkspaceView("settings"));

    expect(screen.queryByRole("region", { name: "Conversation" })).toBeNull();
    expect(control.isConnected).toBe(true);
    expect(document.activeElement).toBe(screen.getByRole("region", { name: "Settings" }));
  });

  it("enters a different main view from a retained Dock child", () => {
    openWorkspaceViewInDock("subagents");
    render(<ChatPanel onSend={() => true} />);
    const child = screen.getByRole("button", { name: "Evidence 0" });
    fireEvent.click(child);
    act(() => child.focus());
    act(() => openWorkspaceView("settings"));

    expect(child.isConnected).toBe(true);
    expect(screen.queryByRole("button", { name: "Evidence 1" })).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("region", { name: "Settings" }));

    fireEvent.click(within(windowHeader()!).getByRole("button", { name: "Close" }));
    expect(screen.getByRole("button", { name: "Evidence 1" })).toBe(child);
  });

  it("enters a main view from the retained panel catalog without losing its query", () => {
    showWorkspaceDock();
    render(<ChatPanel onSend={() => true} />);
    const search = screen.getByRole<HTMLInputElement>("combobox", { name: "Find a panel…" });
    fireEvent.change(search, { target: { value: "Files" } });
    act(() => search.focus());
    act(() => openWorkspaceView("settings"));

    expect(search.isConnected).toBe(true);
    expect(screen.queryByRole("combobox", { name: "Find a panel…" })).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("region", { name: "Settings" }));

    fireEvent.click(within(windowHeader()!).getByRole("button", { name: "Close" }));
    expect(screen.getByRole("combobox", { name: "Find a panel…" })).toBe(search);
    expect(search.value).toBe("Files");
  });

  it("keeps a child that stays visible while its material moves into the Dock", () => {
    openWorkspaceView("file");
    render(<ChatPanel onSend={() => true} />);
    const controls = screen.getByRole("button", { name: "File controls" });
    act(() => controls.focus());
    act(() => openWorkspaceViewInDock("file"));

    expect(navigator().get().view).toBeNull();
    expect(screen.getByRole("complementary", { name: "Files" }).contains(controls)).toBe(true);
    expect(document.activeElement).toBe(controls);
  });

  it("does not replay the handoff while the same main view updates", () => {
    openWorkspaceView("settings");
    const view = render(<ChatPanel onSend={() => true} />);
    const controls = screen.getByRole("button", { name: "Settings controls" });
    act(() => controls.focus());
    view.rerender(<ChatPanel onSend={() => true} />);

    expect(document.activeElement).toBe(controls);
  });
});

describe("Dock collapse focus handoff", () => {
  function retainedDock() {
    openWorkspaceViewInDock("subagents");
    render(
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

      act(() => toggleWorkspaceDock());

      const conversation = screen.getByRole("region", { name: "Conversation" });
      expect(document.activeElement).toBe(conversation);
      expect(dock.isConnected).toBe(true);
      expect(child.isConnected).toBe(true);

      act(() => toggleWorkspaceDock());

      expectRetainedDock(dock, child, draft);
      expect(document.activeElement).toBe(conversation);
    },
  );

  it("keeps a focused Composer and its selection through collapse and reopen", () => {
    const { dock, child, draft } = retainedDock();
    act(() => draft.focus());

    act(() => toggleWorkspaceDock());
    expect(document.activeElement).toBe(draft);

    act(() => toggleWorkspaceDock());
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
      toggleWorkspaceDock();
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

    expect(navigator().get().view).toBeNull();
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

    act(() => toggleWorkspaceDock());
    await settleCleanup();

    expect(search.isConnected).toBe(false);
    const conversation = screen.getByRole("region", { name: "Conversation" });
    expect(document.activeElement).toBe(conversation);
    fireEvent.keyDown(conversation, { key: "Escape" });
    expect(document.activeElement).toBe(conversation);

    act(() => toggleWorkspaceDock());
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

    act(() => toggleWorkspaceDock());
    await settleCleanup();

    expect(menu.isConnected).toBe(false);
    expect(screen.queryByRole("menu")).toBeNull();
    const conversation = screen.getByRole("region", { name: "Conversation" });
    expect(document.activeElement).toBe(conversation);
    fireEvent.keyDown(conversation, { key: "Escape" });
    expect(document.activeElement).toBe(conversation);

    act(() => toggleWorkspaceDock());
    await settleCleanup();

    expect(screen.queryByRole("menu")).toBeNull();
    expectRetainedDock(dock, child, draft);
  });

  it("preserves later outside focus while an open panel picker retires on collapse", async () => {
    retainedDock();
    const later = screen.getByRole("button", { name: "Later control" });
    const search = await openPanelPicker();

    act(() => {
      toggleWorkspaceDock();
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
    expect(navigator().get().view).toBeNull();
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

        act(() => toggleWorkspaceDock());
        await settleCleanup();

        expect(opened.isConnected).toBe(false);
        const conversation = screen.getByRole("region", { name: "Conversation" });
        expect(document.activeElement).toBe(conversation);

        act(() => toggleWorkspaceDock());
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
  openWorkspaceView("main-source");
  render(<ChatPanel onSend={() => true} />);
  const query = screen.getByRole<HTMLInputElement>("searchbox", { name: "Main material query" });
  fireEvent.change(query, { target: { value: "retained source" } });
  const origin = screen.getByRole("button", { name: "src/full-source.ts:8" });
  act(() => origin.focus());
  fireEvent.click(origin);
  expect(navigator().get().view).toBeNull();
  expect(query.isConnected).toBe(true);
  expect(screen.queryByRole("searchbox", { name: "Main material query" })).toBeNull();

  const file = screen.getByRole("button", { name: "File controls" });
  act(() => file.focus());
  fireEvent.keyDown(file, { key: "Escape" });
  expect(navigator().get().view).toBe("main-source");
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
  openWorkspaceView("moved-source");
  render(<ChatPanel onSend={() => true} />);
  const draft = screen.getByRole("textbox", { name: "Moved material draft" });
  fireEvent.click(screen.getByRole("button", { name: "Open in the context dock" }));
  expect(navigator().get().view).toBeNull();
  expect(draft.isConnected).toBe(true);
  fireEvent.keyDown(screen.getByRole("tab", { name: "Moved material" }), { key: "Delete" });
  expect(draft.isConnected).toBe(false);
});

describe("material inspection focus", () => {
  it("clears the local search before returning an inspected material to its source", () => {
    model.views[0] = { ...model.views[0]!, component: FilteredMaterial };
    render(<ChatPanel onSend={() => true} />);
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
      render(<ChatPanel onSend={() => true} />);
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
    render(<ChatPanel onSend={() => true} />);
    const source = screen.getByRole("button", { name: "Inspect task" });
    act(() => source.focus());
    fireEvent.click(source);
    expect(navigator().get().subagent).toBe("child");
    expect(screen.getByRole("button", { name: "Back to conversation" })).toBeTruthy();
    const nested = screen.getByRole("button", { name: "Inspect nested task" });
    act(() => nested.focus());
    fireEvent.click(nested);
    expect(navigator().get().subagent).toBe("nested");
    const file = screen.getByRole("button", { name: "src/child.ts:8" });
    act(() => file.focus());
    fireEvent.click(file);
    fireEvent.click(screen.getByRole("button", { name: "Back to task" }));
    expect(navigator().get()).toMatchObject({ dock: "subagents", subagent: "nested" });
    expect(document.activeElement).toBe(file);
    act(() => screen.getByRole("button", { name: "Read nested" }).focus());
    fireEvent.keyDown(screen.getByRole("complementary", { name: "Subagents" }), {
      key: "Escape",
    });
    expect(navigator().get().subagent).toBe("child");
    expect(document.activeElement).toBe(nested);
    fireEvent.keyDown(screen.getByRole("complementary", { name: "Subagents" }), {
      key: "Escape",
    });
    expect(document.activeElement).toBe(source);
    expect(navigator().get()).toMatchObject({ dock: "subagents", subagent: "child" });
    expect(screen.queryByRole("button", { name: "Back to conversation" })).toBeNull();
  });

  it("retains a filtered catalog and returns to its search control after inspection", () => {
    showWorkspaceDock();
    render(<ChatPanel onSend={() => true} />);
    const search = screen.getByRole("combobox", { name: "Find a panel…" });
    act(() => search.focus());
    fireEvent.change(search, { target: { value: "Files" } });
    fireEvent.click(screen.getByRole("option", { name: "Files" }));
    const dock = screen.getByRole("complementary", { name: "Files" });
    expect(document.activeElement).toBe(dock);
    fireEvent.click(screen.getByRole("button", { name: "Back to panel list" }));

    expect(navigator().get().dock).toBe("catalog");
    expect(screen.getByRole("combobox", { name: "Find a panel…" })).toBe(search);
    expect((search as HTMLInputElement).value).toBe("Files");
    expect(document.activeElement).toBe(search);
    expect(screen.queryByRole("option", { name: "Subagents" })).toBeNull();
  });

  it("enters a picker selection after its popup retires and returns to the previous panel", async () => {
    openWorkspaceViewInDock("subagents");
    render(<ChatPanel onSend={() => true} />);
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

    expect(navigator().get().dock).toBe("subagents");
    expect(document.activeElement).toBe(browse);
    expect(screen.getByRole("button", { name: "Evidence 1" })).toBeTruthy();
  });

  it("moves a full view beside the conversation and restores its named reader on return", () => {
    openWorkspaceView("file");
    render(<ChatPanel onSend={() => true} />);
    fireEvent.click(screen.getByRole("button", { name: "Open in the context dock" }));
    expect(navigator().get().view).toBeNull();
    const dock = screen.getByRole("complementary", { name: "Files" });
    expect(document.activeElement).toBe(dock);
    fireEvent.click(screen.getByRole("button", { name: "Back to Files" }));

    expect(navigator().get().view).toBe("file");
    expect(document.activeElement).toBe(screen.getByRole("region", { name: "Files" }));
  });

  it("enters explicitly opened material and returns to its source without closing the Dock", () => {
    render(<ChatPanel onSend={() => true} />);
    const source = screen.getByRole("button", { name: "src/main.ts:4" });
    act(() => source.focus());
    fireEvent.click(source);
    const dock = screen.getByRole("complementary", { name: "Files" });
    expect(dock.contains(document.activeElement)).toBe(true);
    expect(screen.getByRole("button", { name: "Back to conversation" })).toBeTruthy();

    fireEvent.keyDown(dock, { key: "Escape" });
    expect(document.activeElement).toBe(source);
    expect(navigator().get().dock).toBe("file");
    expect(screen.queryByRole("button", { name: "Back to conversation" })).toBeNull();
  });

  it("returns to the original child reader with its component state intact", () => {
    openWorkspaceViewInDock("subagents");
    render(<ChatPanel onSend={() => true} />);
    fireEvent.click(screen.getByRole("button", { name: "Evidence 0" }));
    const source = screen.getByRole("button", { name: "src/child.ts:8" });
    act(() => source.focus());
    fireEvent.click(source);
    expect(navigator().get().dock).toBe("file");
    expect(document.activeElement).not.toBe(source);
    fireEvent.click(screen.getByRole("button", { name: "Back to Subagents" }));

    expect(navigator().get().dock).toBe("subagents");
    expect(document.activeElement).toBe(source);
    expect(screen.getByRole("button", { name: "Evidence 1" })).toBeTruthy();
  });

  it("keeps focus where the user moved it before entry and during a background update", () => {
    render(<ChatPanel onSend={() => true} />);
    const source = screen.getByRole("button", { name: "src/main.ts:4" });
    const other = screen.getByRole("button", { name: "Other control" });
    act(() => {
      source.focus();
      openWorkspaceFile("src/main.ts", 4, source);
      other.focus();
    });
    expect(document.activeElement).toBe(other);
    act(() => openWorkspaceFile("src/updated.ts"));
    expect(document.activeElement).toBe(other);
  });

  it("retires the source element when the Session changes", () => {
    const view = render(<ChatPanel onSend={() => true} />);
    fireEvent.click(screen.getByRole("button", { name: "src/main.ts:4" }));
    const dock = screen.getByRole("complementary", { name: "Files" });
    expect(within(dock).getByRole("button", { name: "Back to conversation" })).toBeTruthy();
    act(() => {
      model.sessionId = "ses_second";
      activateWorkspaceSessionScope(model.sessionId);
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
    const viewer = renderHook(useWorkspaceFileViewer);
    render(<ChatPanel onSend={() => true} />);
    const { expand, dialog } = await openPreview(conversation());
    const reference = within(dialog).getByRole("button", { name: "src/stream.ts:2" });
    act(() => reference.focus());
    fireEvent.click(reference, { detail: 1 });
    await settle();

    expect(screen.queryByRole("dialog", { name: "Table preview" })).toBeNull();
    expect(viewer.result.current).toEqual({ path: "src/stream.ts", line: 2 });
    expect(navigator().get().dock).toBe("file");
    const dock = screen.getByRole("complementary", { name: "Files" });
    expect(document.activeElement).toBe(dock);

    fireEvent.keyDown(dock, { key: "Escape" });
    expect(document.activeElement).toBe(expand);
  });

  it("returns through a task table preview to the same task and then the conversation", async () => {
    const viewer = renderHook(useWorkspaceFileViewer);
    render(<ChatPanel onSend={() => true} />);
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
    expect(navigator().get()).toMatchObject({ dock: "subagents", subagent: "child" });
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
    openWorkspaceView("main-table");
    render(<ChatPanel onSend={() => true} />);
    const { expand, dialog } = await openPreview(
      screen.getByRole("region", { name: "Table material" }),
    );
    fireEvent.click(within(dialog).getByRole("button", { name: "app/ui/resizer.ts:6" }));
    await settle();
    expect(navigator().get()).toMatchObject({ view: null, dock: "file" });

    fireEvent.keyDown(screen.getByRole("complementary", { name: "Files" }), { key: "Escape" });
    expect(navigator().get().view).toBe("main-table");
    expect(document.activeElement).toBe(expand);
  });

  it("keeps the inline table reference as its own origin", async () => {
    render(<ChatPanel onSend={() => true} />);
    const reference = await within(conversation()).findByRole("button", {
      name: "src/stream.ts:2",
    });
    act(() => reference.focus());
    fireEvent.click(reference, { detail: 1 });
    expect(navigator().get().dock).toBe("file");

    fireEvent.keyDown(screen.getByRole("complementary", { name: "Files" }), { key: "Escape" });
    expect(document.activeElement).toBe(reference);
  });

  it("keeps a selected reference inert without blocking keyboard activation", async () => {
    const viewer = renderHook(useWorkspaceFileViewer);
    render(<ChatPanel onSend={() => true} />);
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
      expect(navigator().get().dock).toBeNull();

      act(() => reference.focus());
      fireEvent.keyDown(reference, { key: "Enter" });
      expect(viewer.result.current).toEqual({ path: "docs/selected.md", line: 9 });
    } finally {
      selection.removeAllRanges();
    }
  });

  it("returns an ordinary close to Expand before and after a navigated preview", async () => {
    render(<ChatPanel onSend={() => true} />);
    let { expand, dialog } = await openPreview(conversation());
    fireEvent.keyDown(dialog, { key: "Escape" });
    await settle();
    expect(screen.queryByRole("dialog", { name: "Table preview" })).toBeNull();
    expect(navigator().get().dock).toBeNull();
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
    const viewer = renderHook(useWorkspaceFileViewer);
    const ratio = renderHook(useDockWidth);
    openWorkspaceViewInDock("file");
    render(<ChatPanel onSend={() => true} />);
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

    const location = navigator().get();
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
    expect(navigator().get()).toEqual(location);
    expect(ratio.result.current.width).toBe(savedRatio);

    fireEvent.keyDown(controls, { key: "Escape" });
    expect(document.activeElement).toBe(source);
    expect(windowHeader()).toBeNull();
    expect(navigator().get()).toMatchObject({ view: null, dock: "file" });
  });

  it("returns through a full-width task, table preview and file to the conversation", async () => {
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(700);
    const viewer = renderHook(useWorkspaceFileViewer);
    render(<ChatPanel onSend={() => true} />);
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
    expect(navigator().get().subagent).toBe("nested");

    const { expand, dialog } = await openPreview(screen.getByRole("region", { name: "Subagents" }));
    const reference = within(dialog).getByRole("button", { name: "README.md:4" });
    act(() => reference.focus());
    fireEvent.keyDown(reference, { key: "Enter" });
    await settle();
    expect(viewer.result.current).toEqual({ path: "README.md", line: 4 });
    expect(document.activeElement).toBe(screen.getByRole("region", { name: "Files" }));

    fireEvent.click(within(windowHeader()!).getByRole("button", { name: "Back to task" }));
    expect(navigator().get()).toMatchObject({ view: null, dock: "subagents", subagent: "nested" });
    expect(document.activeElement).toBe(expand);
    fireEvent.keyDown(within(windowHeader()!).getByRole("button", { name: "Back to task" }), {
      key: "Escape",
    });
    expect(navigator().get()).toMatchObject({ view: null, subagent: "child" });
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
      render(<ChatPanel onSend={() => true} />);
      const source = screen.getByRole("button", { name: "Inspect task" });
      act(() => source.focus());
      fireEvent.click(source);
      act(() => openWorkspaceView("subagents"));
      expect(navigator().get()).toMatchObject({ view: "subagents", subagent: "child" });
      let header = within(windowHeader()!);
      expect(header.getByRole("button", { name: "Back to conversation" })).toBeTruthy();
      expect(header.queryByRole("button", { name: "Close" })).toBeNull();

      fireEvent.click(screen.getByRole("button", { name: "Evidence 0" }));
      const file = screen.getByRole("button", { name: "src/child.ts:8" });
      act(() => file.focus());
      fireEvent.click(file);
      expect(navigator().get()).toMatchObject({ view: null, dock: "file" });
      fireEvent.click(screen.getByRole("button", { name: "Back to task" }));

      expect(navigator().get()).toMatchObject({ view: "subagents", subagent: "child" });
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
      expect(navigator().get().view).toBeNull();
      expect(windowHeader()).toBeNull();
      expect(screen.queryByRole("button", { name: "Back to conversation" })).toBeNull();
    },
  );

  it("leaves outside, composing and local Escape to their owners before returning", () => {
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(700);
    model.views[0] = { ...model.views[0]!, component: FilteredMaterial };
    render(
      <>
        <button>Outside control</button>
        <ChatPanel onSend={() => true} />
      </>,
    );
    const outside = screen.getByRole("button", { name: "Outside control" });
    const source = screen.getByRole("button", { name: "src/main.ts:4" });
    act(() => {
      source.focus();
      openWorkspaceFile("src/main.ts", 4, source);
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
    openWorkspaceViewInDock("subagents");
    render(<ChatPanel onSend={() => true} />);
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
    showWorkspaceDock();
    render(<ChatPanel onSend={() => true} />);
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
    openWorkspaceView("file");
    render(
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
      navigator().back();
      later.focus();
    });
    await settle();

    expect(navigator().get().view).toBe("file");
    expect(screen.queryByRole("dialog", { name: "Table preview" })).toBeNull();
    expect(document.activeElement).toBe(later);
  });

  it("retires a retained main task preview when history presents another material", async () => {
    openWorkspaceViewInDock("subagents");
    openWorkspaceView("file");
    openWorkspaceView("subagents");
    render(<ChatPanel onSend={() => true} />);
    const source = screen.getByRole("button", { name: "Evidence 0" });
    const trigger = screen.getByRole("button", { name: "Expand table" });
    act(() => trigger.focus());
    fireEvent.click(trigger);
    await settle();

    act(() => navigator().back());
    await settle();

    expect(screen.queryByRole("dialog", { name: "Table preview" })).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("region", { name: "Files" }));
    act(() => navigator().forward());
    await settle();
    expect(screen.queryByRole("dialog", { name: "Table preview" })).toBeNull();
    expect(screen.getByRole("button", { name: "Evidence 0" })).toBe(source);
    expect(screen.getByRole("button", { name: "Expand table" })).toBe(trigger);
    fireEvent.click(trigger);
    expect(screen.getByRole("dialog", { name: "Table preview" })).toBeTruthy();
  });

  it("retires a table preview when history hides its conversation", async () => {
    openWorkspaceView("file");
    render(<ChatPanel onSend={() => true} />);
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    const draft = screen.getByLabelText<HTMLTextAreaElement>("Composer draft");
    const trigger = screen.getByRole("button", { name: "Expand table" });
    act(() => trigger.focus());
    fireEvent.click(trigger);
    await settle();
    expect(screen.getByRole("dialog", { name: "Table preview" })).toBeTruthy();
    act(() => navigator().back());
    await settle();
    expect(navigator().get().view).toBe("file");
    expect(screen.queryByRole("dialog", { name: "Table preview" })).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("region", { name: "Files" }));
    act(() => navigator().forward());
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
      openWorkspaceViewInDock("subagents");
      render(<ChatPanel onSend={() => true} />);
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

      if (transition === "collapse") act(() => toggleWorkspaceDock());
      else await resize(700);

      await vi.waitFor(() => {
        expect(screen.queryByRole(kind === "select" ? "listbox" : "menu")).toBeNull();
        expect(document.activeElement).toBe(screen.getByRole("region", { name: "Conversation" }));
      });
      expect(child.isConnected).toBe(true);
      expect(select).not.toHaveBeenCalled();
      expect(command).not.toHaveBeenCalled();

      if (transition === "collapse") act(() => toggleWorkspaceDock());
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
      openWorkspaceViewInDock("subagents");
      render(<ChatPanel onSend={() => true} />);
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

      if (transition === "collapse") act(() => toggleWorkspaceDock());
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
          within(conversation).getByRole("button", { name: "src/main.ts:4" }),
        ),
      );

      if (transition === "collapse") act(() => toggleWorkspaceDock());
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

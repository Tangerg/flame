import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { useState } from "react";
import { SearchField } from "@flame/ui";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkspaceViewSpec } from "@/plugins/sdk";
import { navigator } from "@/lib/navigation";
import { FileRefLink } from "@/plugins/builtin/chat/file-references/public/FileRefLink";
import {
  activateWorkspaceSessionScope,
  openWorkspaceFile,
  openWorkspaceSubagentRun,
  openWorkspaceView,
  openWorkspaceViewInDock,
  showWorkspaceDock,
  useWorkspaceSubagentRunId,
} from "@/plugins/builtin/workspace/public/navigation";
import { ChatPanel } from "./ChatPanel";

const model = vi.hoisted(() => ({ sessionId: "ses_first", views: [] as WorkspaceViewSpec[] }));
vi.mock("@/plugins/builtin/agent/public/session", () => ({
  useActiveSession: () => null,
  useActiveSessionId: () => model.sessionId,
  useAgentSessions: () => ({ isLoading: false }),
}));
vi.mock("@/plugins/builtin/workspace/public/contextDockCatalog", () => ({
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
vi.mock("@/plugins/sdk", async (original) => ({
  ...(await original<typeof import("@/plugins/sdk")>()),
  useWorkspaceViews: () => model.views,
}));
vi.mock("@/plugins/host/Slot", () => ({ Slot: () => null }));
vi.mock("./HeaderDiffStat", () => ({ HeaderDiffStat: () => null }));
vi.mock("./RunStatusPill", () => ({ RunStatusPill: () => null }));
vi.mock("./RunAnnouncer", () => ({ RunAnnouncer: () => null }));
vi.mock("./ChatStream", async () => {
  const { FileRefLink } = await import("@/plugins/builtin/chat/file-references/public/FileRefLink");
  return {
    ChatStream: () => (
      <>
        <FileRefLink path="src/main.ts" line={4} />
        <button onClick={(event) => openWorkspaceSubagentRun("child", event.currentTarget)}>
          Inspect task
        </button>
        <button>Other control</button>
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
    </>
  );
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

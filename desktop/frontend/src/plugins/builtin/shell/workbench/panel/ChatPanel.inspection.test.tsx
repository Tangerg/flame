import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkspaceViewSpec } from "@/plugins/sdk";
import { navigator } from "@/lib/navigation";
import { FileRefLink } from "@/plugins/builtin/chat/file-references/public/FileRefLink";
import {
  activateWorkspaceSessionScope,
  openWorkspaceFile,
  openWorkspaceView,
  openWorkspaceViewInDock,
  showWorkspaceDock,
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
        <button>Other control</button>
      </>
    ),
  };
});

function ChildReader() {
  const [count, setCount] = useState(0);
  return (
    <>
      <button onClick={() => setCount(count + 1)}>Evidence {count}</button>
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

describe("material inspection focus", () => {
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

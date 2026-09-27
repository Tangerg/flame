import { cleanup, render, screen } from "@testing-library/react";
import { useState, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkspaceViewSpec } from "@/plugins/sdk";
import { ChatPanel } from "./ChatPanel";

const model = vi.hoisted(() => ({
  setWidth: vi.fn(),
  width: 0.5 as number | null,
  sessionId: "ses_first",
  activeMainView: "stateful" as string | null,
  dock: { open: false, viewIds: [] as string[], activeViewId: null as string | null },
  isLoading: false,
  views: [] as WorkspaceViewSpec[],
}));

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  useActiveSession: () => null,
  useActiveSessionId: () => model.sessionId,
  useAgentSessions: () => ({ isLoading: model.isLoading }),
}));

vi.mock("@/plugins/builtin/agent/public/run", () => ({
  useIsCurrentRootRunning: () => false,
  useCurrentRootMaterial: () => ({ status: "idle", outcome: null }),
}));

vi.mock("@/plugins/builtin/workspace/public/navigation", () => ({
  WORKSPACE_DOCK_CATALOG: "catalog",
  closeAllWorkspaceDockViews: vi.fn(),
  closeOtherWorkspaceDockViews: vi.fn(),
  closeWorkspaceDockView: vi.fn(),
  closeWorkspaceView: vi.fn(),
  collapseWorkspaceDock: vi.fn(),
  openWorkspaceViewInDock: vi.fn(),
  reorderWorkspaceDockView: vi.fn(),
  selectWorkspaceDockView: vi.fn(),
  showWorkspaceDock: vi.fn(),
  useActiveWorkspaceViewId: () => model.activeMainView,
  useWorkspaceDock: () => model.dock,
}));

vi.mock("@/plugins/builtin/workspace/public/contextDockCatalog", () => ({
  useContextDockCatalog: () => [],
}));

vi.mock("@/plugins/builtin/workspace/public/sidebarDrawer", () => ({
  useDockWidth: () => ({ width: model.width, setWidth: model.setWidth }),
}));

vi.mock("@/plugins/sdk", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/plugins/sdk")>()),
  useWorkspaceViews: () => model.views,
}));

vi.mock("@/plugins/host/PluginBoundary", () => ({
  PluginBoundary: ({ children }: { children: ReactNode }) => children,
}));

vi.mock("@/plugins/host/Slot", () => ({ Slot: () => null }));
vi.mock("./ChatStream", () => ({ ChatStream: () => null }));
vi.mock("./HeaderDiffStat", () => ({ HeaderDiffStat: () => null }));

let nextInstance = 0;

function StatefulWorkspaceView() {
  const [instance] = useState(() => ++nextInstance);
  return <span>workspace-instance:{instance}</span>;
}

beforeEach(() => {
  model.isLoading = false;
  model.width = 0.5;
  model.setWidth.mockClear();
});

afterEach(() => cleanup());

describe("ChatPanel Session-owned workspace view state", () => {
  it("retires a promoted workspace view when the exact Session changes", () => {
    nextInstance = 0;
    model.sessionId = "ses_first";
    model.activeMainView = "stateful";
    model.dock = { open: false, viewIds: [], activeViewId: null };
    model.views = [
      {
        id: "stateful",
        title: "stateful",
        icon: "tool",
        component: StatefulWorkspaceView,
      },
    ];
    const view = render(<ChatPanel onSend={() => true} />);
    expect(screen.getByText("workspace-instance:1")).toBeTruthy();

    model.sessionId = "ses_second";
    view.rerender(<ChatPanel onSend={() => true} />);

    expect(screen.getByText("workspace-instance:2")).toBeTruthy();
  });

  it("keeps the dock on the same Session ownership boundary", () => {
    nextInstance = 0;
    model.sessionId = "ses_first";
    model.activeMainView = null;
    model.dock = { open: true, viewIds: ["stateful"], activeViewId: "stateful" };
    model.views = [
      {
        id: "stateful",
        title: "stateful",
        icon: "tool",
        component: StatefulWorkspaceView,
      },
    ];
    const view = render(<ChatPanel onSend={() => true} />);
    expect(screen.getByText("workspace-instance:1")).toBeTruthy();

    model.sessionId = "ses_second";
    view.rerender(<ChatPanel onSend={() => true} />);

    expect(screen.getByText("workspace-instance:2")).toBeTruthy();
  });

  it("does not mount unowned dock material without an active Session", () => {
    nextInstance = 0;
    model.sessionId = "";
    model.activeMainView = null;
    model.dock = { open: true, viewIds: ["stateful"], activeViewId: "stateful" };
    model.views = [
      {
        id: "stateful",
        title: "stateful",
        icon: "tool",
        component: StatefulWorkspaceView,
      },
    ];

    const view = render(<ChatPanel onSend={() => true} />);

    expect(screen.queryByText(/workspace-instance:/)).toBeNull();
    expect(view.container.querySelector('[data-dock="open"]')).toBeNull();
  });

  it("reconciles dock availability after the loading placeholder yields to the shell", () => {
    model.sessionId = "ses_first";
    model.activeMainView = null;
    model.dock = { open: false, viewIds: [], activeViewId: null };
    model.views = [];
    model.isLoading = true;

    const view = render(<ChatPanel onSend={() => true} />);
    expect(view.container.firstChild).toBeNull();

    model.isLoading = false;
    view.rerender(<ChatPanel onSend={() => true} />);

    expect(
      screen.getByRole<HTMLButtonElement>("button", {
        name: /Open material full width/,
      }).disabled,
    ).toBe(false);
  });
});

it("measures dock availability when returning from a promoted main view", () => {
  let rowWidth = 600;
  const width = vi
    .spyOn(HTMLElement.prototype, "clientWidth", "get")
    .mockImplementation(() => rowWidth);
  try {
    model.width = null;
    model.sessionId = "ses_first";
    model.activeMainView = "stateful";
    model.dock = { open: false, viewIds: [], activeViewId: null };
    model.views = [
      { id: "stateful", title: "stateful", icon: "tool", component: StatefulWorkspaceView },
    ];
    const view = render(<ChatPanel onSend={() => true} />);
    model.activeMainView = null;
    view.rerender(<ChatPanel onSend={() => true} />);
    expect(
      screen.getByRole<HTMLButtonElement>("button", {
        name: /Open material full width/,
      }).disabled,
    ).toBe(false);
    expect(model.setWidth).not.toHaveBeenCalled();
    model.activeMainView = "stateful";
    view.rerender(<ChatPanel onSend={() => true} />);
    rowWidth = 1440;
    model.activeMainView = null;
    view.rerender(<ChatPanel onSend={() => true} />);
    expect(
      screen.getByRole<HTMLButtonElement>("button", { name: "Open right workspace" }).disabled,
    ).toBe(false);
    expect(model.setWidth).not.toHaveBeenCalled();
  } finally {
    cleanup();
    width.mockRestore();
  }
});

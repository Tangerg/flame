import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { lazy, useState, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkspaceViewSpec } from "@/plugins/sdk";
import {
  closeWorkspaceView,
  openWorkspaceViewInDock,
} from "@/plugins/builtin/workspace/public/navigation";
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
  useWorkspaceInspection: () => null,
  returnFromWorkspaceInspection: vi.fn(),
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

describe("ChatPanel full-view window controls", () => {
  beforeEach(() => {
    model.sessionId = "ses_window";
    model.activeMainView = "current-view";
    model.dock = { open: false, viewIds: [], activeViewId: null };
    model.views = [];
    vi.mocked(closeWorkspaceView).mockClear();
    vi.mocked(openWorkspaceViewInDock).mockClear();
  });

  function expectWindowHeader(container: HTMLElement, title: string) {
    const header = container.querySelector<HTMLElement>('[data-slot="workspace-window-header"]');
    expect(header).not.toBeNull();
    expect(header!.closest('[data-slot="workspace-view-body"]')).toBeNull();
    const controls = within(header!);
    expect(controls.getByText(title, { exact: true })).toBeTruthy();
    expect(controls.getByRole<HTMLButtonElement>("button", { name: "Close" }).disabled).toBe(false);
    return controls;
  }

  it("keeps the title and Close available while a lazy view is suspended and after it loads", async () => {
    const loading = Promise.withResolvers<{ default: () => ReactNode }>();
    model.views = [
      {
        id: "current-view",
        title: "Suspended workspace",
        component: lazy(() => loading.promise),
      },
    ];
    model.isLoading = true;
    const view = render(<ChatPanel onSend={() => true} />);

    expect(screen.queryByText("Loaded workspace")).toBeNull();
    let controls = expectWindowHeader(view.container, "Suspended workspace");
    fireEvent.click(controls.getByRole("button", { name: "Close" }));
    expect(closeWorkspaceView).toHaveBeenLastCalledWith("current-view");

    await act(async () => {
      loading.resolve({ default: () => <p>Loaded workspace</p> });
    });

    expect(screen.getByText("Loaded workspace")).toBeTruthy();
    controls = expectWindowHeader(view.container, "Suspended workspace");
    fireEvent.click(controls.getByRole("button", { name: "Close" }));
    expect(closeWorkspaceView).toHaveBeenCalledTimes(2);
    expect(closeWorkspaceView).toHaveBeenLastCalledWith("current-view");
  });

  it("keeps window controls outside a failed view and its real boundary retry", () => {
    const failure = new Error("The workspace fixture failed to render");
    let failing = true;
    function RecoveringWorkspaceView() {
      if (failing) throw failure;
      return <p>Recovered workspace</p>;
    }
    model.views = [
      { id: "current-view", title: "Recoverable workspace", component: RecoveringWorkspaceView },
    ];
    const logError = console.error.bind(console);
    const errorLog = vi.spyOn(console, "error").mockImplementation((...args: unknown[]) => {
      if (!args.includes(failure)) logError(...args);
    });
    try {
      const view = render(<ChatPanel onSend={() => true} />);
      expect(screen.getByRole("alert").textContent).toContain(failure.message);
      let controls = expectWindowHeader(view.container, "Recoverable workspace");
      fireEvent.click(controls.getByRole("button", { name: "Close" }));
      expect(closeWorkspaceView).toHaveBeenLastCalledWith("current-view");

      failing = false;
      fireEvent.click(screen.getByRole("button", { name: "Retry" }));

      expect(screen.queryByRole("alert")).toBeNull();
      expect(screen.getByText("Recovered workspace")).toBeTruthy();
      controls = expectWindowHeader(view.container, "Recoverable workspace");
      fireEvent.click(controls.getByRole("button", { name: "Close" }));
      expect(closeWorkspaceView).toHaveBeenCalledTimes(2);
      expect(closeWorkspaceView).toHaveBeenLastCalledWith("current-view");
    } finally {
      errorLog.mockRestore();
    }
  });

  it("offers Close for an unknown view without offering a context dock target", () => {
    model.activeMainView = "missing-view";
    const view = render(<ChatPanel onSend={() => true} />);
    const controls = expectWindowHeader(view.container, "View unavailable");

    expect(controls.queryByRole("button", { name: "Open in the context dock" })).toBeNull();
    fireEvent.click(controls.getByRole("button", { name: "Close" }));
    expect(closeWorkspaceView).toHaveBeenCalledExactlyOnceWith("missing-view");
  });

  it.each([undefined, "workspace", "session", "run"] as const)(
    "offers context dock navigation only for a declared dock scope (%s)",
    (dock) => {
      model.views = [
        { id: "current-view", title: "Current workspace", dock, component: StatefulWorkspaceView },
      ];
      const view = render(<ChatPanel onSend={() => true} />);
      let controls = expectWindowHeader(view.container, "Current workspace");
      const openBeside = controls.queryByRole("button", {
        name: "Open in the context dock",
      });
      expect(openBeside !== null).toBe(dock !== undefined);
      if (openBeside) fireEvent.click(openBeside);
      expect(vi.mocked(openWorkspaceViewInDock).mock.calls).toEqual(
        dock === undefined ? [] : [["current-view"]],
      );

      model.activeMainView = "replacement-view";
      model.views = [
        {
          id: "replacement-view",
          title: "Replacement workspace",
          component: StatefulWorkspaceView,
        },
      ];
      view.rerender(<ChatPanel onSend={() => true} />);
      controls = expectWindowHeader(view.container, "Replacement workspace");
      expect(controls.queryByRole("button", { name: "Open in the context dock" })).toBeNull();
      fireEvent.click(controls.getByRole("button", { name: "Close" }));
      expect(closeWorkspaceView).toHaveBeenCalledExactlyOnceWith("replacement-view");
    },
  );
});

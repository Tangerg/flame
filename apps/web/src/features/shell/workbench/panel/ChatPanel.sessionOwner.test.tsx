import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen, within } from "@testing-library/react";
import { lazy, useState, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi, type MockInstance } from "vitest";
import { PanelHeader } from "@flame/ui/panel";
import { type WorkspaceViewSpec } from "@/features/workspace/public/views";
import {
  closeWorkspaceView,
  openWorkspaceViewInDock,
} from "@/features/workspace/public/navigation";
import { ChatPanel } from "@/features/shell/workbench/panel/ChatPanel";
const model = vi.hoisted(() => ({
  setWidth: vi.fn(),
  width: 0.5 as number | null,
  sessionId: "ses_first",
  activeMainView: "stateful" as string | null,
  dock: { open: false, viewIds: [] as string[], activeViewId: null as string | null },
  isLoading: false,
  views: [] as WorkspaceViewSpec[],
}));

vi.mock("@/features/agent/public/session", () => ({
  useActiveSession: () => null,
  useActiveSessionId: () => model.sessionId,
  useAgentSessions: () => ({ isLoading: model.isLoading, data: model.isLoading ? undefined : [] }),
}));

vi.mock("@/features/agent/public/run", () => ({
  useIsCurrentRootRunning: () => false,
  useCurrentRootMaterial: () => ({ status: "idle", outcome: null }),
}));

vi.mock("@/features/workspace/public/navigation", () => ({
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
  useWorkspaceSubagentRunId: () => null,
  returnFromWorkspaceInspection: vi.fn(),
}));

vi.mock("@/features/workspace/public/contextDockCatalog", () => ({
  useContextDockCatalog: () => [],
}));

vi.mock("@/features/workspace/public/sidebarDrawer", () => ({
  useDockWidth: () => ({ width: model.width, setWidth: model.setWidth }),
}));

vi.mock("@/features/workspace/public/views", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/features/workspace/public/views")>();
  return { ...actual, useWorkspaceViews: () => model.views };
});

vi.mock("@/extensions/host/Slot", () => ({ Slot: () => null }));

vi.mock("@/features/shell/workbench/panel/ChatStream", () => ({ ChatStream: () => null }));

vi.mock("@/features/shell/workbench/panel/HeaderDiffStat", () => ({ HeaderDiffStat: () => null }));

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
    const view = testClient.render(<ChatPanel onSend={() => true} />);
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
    const view = testClient.render(<ChatPanel onSend={() => true} />);
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

    const view = testClient.render(<ChatPanel onSend={() => true} />);

    expect(screen.queryByText(/workspace-instance:/)).toBeNull();
    expect(view.container.querySelector('[data-dock="open"]')).toBeNull();
  });

  it("does not mount retained Session material while the first read is pending", () => {
    nextInstance = 0;
    model.sessionId = "ses_first";
    model.activeMainView = null;
    model.dock = { open: false, viewIds: ["stateful"], activeViewId: "stateful" };
    model.views = [{ id: "stateful", title: "stateful", component: StatefulWorkspaceView }];
    model.isLoading = true;

    const view = testClient.render(<ChatPanel onSend={() => true} />);
    expect(screen.queryByText(/workspace-instance:/)).toBeNull();
    model.isLoading = false;
    view.rerender(<ChatPanel onSend={() => true} />);
    expect(screen.getByText("workspace-instance:1")).toBeTruthy();
  });

  it("keeps pending connection layout measurable before Session material arrives", () => {
    model.sessionId = "ses_first";
    model.activeMainView = null;
    model.dock = { open: false, viewIds: [], activeViewId: null };
    model.views = [];
    model.isLoading = true;

    const view = testClient.render(<ChatPanel onSend={() => true} />);
    expect(screen.getByRole("main", { name: "Agent workspace" })).toBeTruthy();
    expect(view.container.querySelector('[data-slot="composer-overlay"]')).toBeNull();
    expect(screen.queryByRole("button", { name: /Open material full width/ })).toBeNull();

    model.isLoading = false;
    view.rerender(<ChatPanel onSend={() => true} />);

    expect(
      screen.getByRole<HTMLButtonElement>("button", {
        name: /Open material full width/,
      }).disabled,
    ).toBe(false);
  });
});

it("switches one mounted view between dock and full-width density under host identity", () => {
  const width = vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(1200);
  try {
    nextInstance = 0;
    model.sessionId = "ses_first";
    model.activeMainView = null;
    model.dock = { open: true, viewIds: ["stateful"], activeViewId: "stateful" };
    model.views = [
      {
        id: "stateful",
        title: "stateful",
        icon: "tool",
        component: () => (
          <>
            <PanelHeader title="Density heading" />
            <StatefulWorkspaceView />
          </>
        ),
      },
    ];
    const view = testClient.render(<ChatPanel onSend={() => true} />);
    const windowHeader = () =>
      view.container.querySelector<HTMLElement>('[data-slot="workspace-window-header"]');
    const heading = screen.getByRole("heading", { name: "Density heading" });
    const header = heading.closest("header")!;
    expect(header.classList.contains("sr-only")).toBe(true);
    expect(windowHeader()).toBeNull();
    expect(screen.getByText("workspace-instance:1")).toBeTruthy();

    model.activeMainView = "stateful";
    view.rerender(<ChatPanel onSend={() => true} />);
    expect(within(windowHeader()!).getByText("stateful", { exact: true })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Density heading" })).toBe(heading);
    expect(header.classList.contains("sr-only")).toBe(true);
    expect(screen.getByText("workspace-instance:1")).toBeTruthy();

    model.activeMainView = null;
    view.rerender(<ChatPanel onSend={() => true} />);
    expect(windowHeader()).toBeNull();
    expect(screen.getByRole("heading", { name: "Density heading" })).toBe(heading);
    expect(header.classList.contains("sr-only")).toBe(true);
    expect(screen.getByText("workspace-instance:1")).toBeTruthy();
  } finally {
    cleanup();
    width.mockRestore();
  }
});

it("keeps measuring dock availability while a promoted main view is presented", () => {
  let rowWidth = 600;
  const width = vi
    .spyOn(HTMLElement.prototype, "clientWidth", "get")
    .mockImplementation(() => rowWidth);
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
  try {
    model.width = null;
    model.sessionId = "ses_first";
    model.activeMainView = "stateful";
    model.dock = { open: false, viewIds: [], activeViewId: null };
    model.views = [
      { id: "stateful", title: "stateful", icon: "tool", component: StatefulWorkspaceView },
    ];
    const view = testClient.render(<ChatPanel onSend={() => true} />);
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
    act(() => observers.forEach((observe) => observe([], {} as ResizeObserver)));
    model.activeMainView = null;
    view.rerender(<ChatPanel onSend={() => true} />);
    expect(
      screen.getByRole<HTMLButtonElement>("button", { name: "Open right workspace" }).disabled,
    ).toBe(false);
    expect(model.setWidth).not.toHaveBeenCalled();
  } finally {
    cleanup();
    vi.unstubAllGlobals();
    width.mockRestore();
  }
});

it("projects the saved ratio onto the observed row width without saving a new ratio", async () => {
  let rowWidth = 1440;
  const width = vi
    .spyOn(HTMLElement.prototype, "clientWidth", "get")
    .mockImplementation(() => rowWidth);
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
  try {
    nextInstance = 0;
    model.width = 0.5;
    model.sessionId = "ses_first";
    model.activeMainView = null;
    model.dock = { open: true, viewIds: ["stateful"], activeViewId: "stateful" };
    model.views = [
      { id: "stateful", title: "stateful", icon: "tool", component: StatefulWorkspaceView },
    ];
    const view = testClient.render(<ChatPanel onSend={() => true} />);
    const row = view.container.querySelector<HTMLElement>("[data-dock]")!;
    const measure = () => row.style.getPropertyValue("--dock-measure");
    expect(row.dataset.dock).toBe("open");
    expect(measure()).toBe("660px");

    rowWidth = 1000;
    act(() => observers.forEach((observe) => observe([], {} as ResizeObserver)));
    expect(measure()).toBe("440px");

    model.width = null;
    view.rerender(<ChatPanel onSend={() => true} />);
    expect(measure()).toBe("480px");

    rowWidth = 700;
    observers.forEach((observe) => observe([], {} as ResizeObserver));
    await act(() => new Promise((resolve) => requestAnimationFrame(resolve)));
    expect(row.dataset.dock).toBe("collapsed");
    expect(screen.getByText("workspace-instance:1")).toBeTruthy();
    expect(model.setWidth).not.toHaveBeenCalled();
  } finally {
    cleanup();
    vi.unstubAllGlobals();
    width.mockRestore();
  }
});

describe("ChatPanel full-view window controls", () => {
  let rowWidth: MockInstance<() => number>;
  beforeEach(() => {
    rowWidth = vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(1200);
    model.sessionId = "ses_window";
    model.activeMainView = "current-view";
    model.dock = { open: false, viewIds: [], activeViewId: null };
    model.views = [];
    vi.mocked(closeWorkspaceView).mockClear();
    vi.mocked(openWorkspaceViewInDock).mockClear();
  });
  afterEach(() => rowWidth.mockRestore());

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
    const view = testClient.render(<ChatPanel onSend={() => true} />);

    expect(screen.queryByText("Loaded workspace")).toBeNull();
    let controls = expectWindowHeader(view.container, "Suspended workspace");
    fireEvent.click(controls.getByRole("button", { name: "Close" }));
    expect(closeWorkspaceView).toHaveBeenLastCalledWith(
      testClient.workspace.access,
      "current-view",
    );

    await act(async () => {
      loading.resolve({ default: () => <p>Loaded workspace</p> });
    });

    expect(screen.getByText("Loaded workspace")).toBeTruthy();
    controls = expectWindowHeader(view.container, "Suspended workspace");
    fireEvent.click(controls.getByRole("button", { name: "Close" }));
    expect(closeWorkspaceView).toHaveBeenCalledTimes(2);
    expect(closeWorkspaceView).toHaveBeenLastCalledWith(
      testClient.workspace.access,
      "current-view",
    );
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
      const view = testClient.render(<ChatPanel onSend={() => true} />);
      expect(screen.getByRole("alert").textContent).toContain(failure.message);
      let controls = expectWindowHeader(view.container, "Recoverable workspace");
      fireEvent.click(controls.getByRole("button", { name: "Close" }));
      expect(closeWorkspaceView).toHaveBeenLastCalledWith(
        testClient.workspace.access,
        "current-view",
      );

      failing = false;
      fireEvent.click(screen.getByRole("button", { name: "Retry" }));

      expect(screen.queryByRole("alert")).toBeNull();
      expect(screen.getByText("Recovered workspace")).toBeTruthy();
      controls = expectWindowHeader(view.container, "Recoverable workspace");
      fireEvent.click(controls.getByRole("button", { name: "Close" }));
      expect(closeWorkspaceView).toHaveBeenCalledTimes(2);
      expect(closeWorkspaceView).toHaveBeenLastCalledWith(
        testClient.workspace.access,
        "current-view",
      );
    } finally {
      errorLog.mockRestore();
    }
  });

  it("offers Close for an unknown view without offering a context dock target", () => {
    model.activeMainView = "missing-view";
    const view = testClient.render(<ChatPanel onSend={() => true} />);
    const controls = expectWindowHeader(view.container, "View unavailable");

    expect(controls.queryByRole("button", { name: "Open in the context dock" })).toBeNull();
    fireEvent.click(controls.getByRole("button", { name: "Close" }));
    expect(closeWorkspaceView).toHaveBeenCalledExactlyOnceWith(
      testClient.workspace.access,
      "missing-view",
    );
  });

  it.each([undefined, "workspace", "session", "run"] as const)(
    "offers context dock navigation only for a declared dock scope (%s)",
    (dock) => {
      model.views = [
        { id: "current-view", title: "Current workspace", dock, component: StatefulWorkspaceView },
      ];
      const view = testClient.render(<ChatPanel onSend={() => true} />);
      let controls = expectWindowHeader(view.container, "Current workspace");
      const openBeside = controls.queryByRole("button", {
        name: "Open in the context dock",
      });
      expect(openBeside !== null).toBe(dock !== undefined);
      if (openBeside) fireEvent.click(openBeside);
      expect(vi.mocked(openWorkspaceViewInDock).mock.calls).toEqual(
        dock === undefined ? [] : [[testClient.workspace.access, "current-view"]],
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
      expect(closeWorkspaceView).toHaveBeenCalledExactlyOnceWith(
        testClient.workspace.access,
        "replacement-view",
      );
    },
  );
});

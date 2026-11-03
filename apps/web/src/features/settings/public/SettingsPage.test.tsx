import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { definePlugin } from "@/extensions/definePlugin";
import { usePluginErrorStore } from "@/extensions/errors";
import { SETTINGS_PANE } from "@/features/settings/public/panes";
import { drainBrowserTasks } from "@/test/browserTasks";
import { SettingsPage } from "@/features/settings/public/SettingsPage";
import { loadPluginsForTest } from "@/test/kernel";

async function loadPanes() {
  await loadPluginsForTest(
    definePlugin({
      name: "test.settings",
      setup: (ctx) => {
        ctx.contribute(SETTINGS_PANE, {
          id: "appearance",
          label: "Appearance",
          description: "Tune the interface",
          order: 0,
          component: () => <div data-testid="appearance-body">appearance body</div>,
        });
        ctx.contribute(SETTINGS_PANE, {
          id: "plugins",
          label: "Plugins",
          keywords: ["Hot reload"],
          order: 10,
          component: () => <div data-testid="plugins-body">plugins body</div>,
        });
      },
    }),
  );
}

async function loadRailPanes() {
  await loadPluginsForTest(
    definePlugin({
      name: "test.settings.rail",
      setup: (ctx) => {
        const panes: { id: string; label: string; group: string; keywords?: string[] }[] = [
          { id: "general", label: "General", group: "general" },
          { id: "appearance", label: "Appearance", group: "general" },
          { id: "providers", label: "Providers", group: "models" },
          { id: "approvals", label: "Approvals", group: "agent" },
          { id: "mcp", label: "MCP servers", group: "integrations" },
          { id: "plugins", label: "Plugins", group: "extensions", keywords: ["Hot reload"] },
        ];
        panes.forEach((pane, order) =>
          ctx.contribute(SETTINGS_PANE, {
            ...pane,
            order,
            component: () =>
              pane.id === "mcp" ? (
                <input aria-label="Description" defaultValue="workspace tools draft" />
              ) : (
                <div>{pane.label} body</div>
              ),
          }),
        );
      },
    }),
  );
}

interface Observed {
  callback: ResizeObserverCallback;
  observer: ResizeObserver;
  targets: Set<Element>;
}

function stubResizeObservers() {
  const observed: Observed[] = [];
  class FakeResizeObserver {
    private entry: Observed;
    constructor(callback: ResizeObserverCallback) {
      this.entry = { callback, observer: this as unknown as ResizeObserver, targets: new Set() };
      observed.push(this.entry);
    }
    observe(target: Element) {
      this.entry.targets.add(target);
    }
    unobserve(target: Element) {
      this.entry.targets.delete(target);
    }
    disconnect() {
      this.entry.targets.clear();
    }
  }
  vi.stubGlobal("ResizeObserver", FakeResizeObserver);
  const queuedFor = (target: Element) =>
    observed
      .filter((entry) => entry.targets.has(target))
      .map((entry) => () => entry.callback([{ target } as ResizeObserverEntry], entry.observer));
  return {
    queuedFor,
    resize: (target: Element) => act(() => queuedFor(target).forEach((deliver) => deliver())),
  };
}

// Rail frame starts at 100 with a 4px border; each tab is 32px tall on a 40px pitch.
function mockRailGeometry(list: HTMLElement, frame: { height: number }) {
  let scrollTop = 0;
  Object.defineProperty(list, "clientTop", { configurable: true, get: () => 4 });
  Object.defineProperty(list, "clientHeight", { configurable: true, get: () => frame.height });
  Object.defineProperty(list, "scrollTop", {
    configurable: true,
    get: () => scrollTop,
    set: (next: number) => {
      scrollTop = next;
    },
  });
  vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(function (this: Element) {
    if (this === list) return new DOMRect(0, 100, 200, frame.height + 8);
    const index = Array.from(list.querySelectorAll('[role="tab"]')).indexOf(this);
    if (index < 0) return new DOMRect();
    return new DOMRect(0, 104 + 40 * index - scrollTop, 200, 32);
  });
}

describe("settingsPage", () => {
  afterEach(async () => {
    cleanup();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    await drainBrowserTasks();
  });

  it("shows the first pane by default and renders its body", async () => {
    await loadPanes();
    testClient.render(<SettingsPage />);
    expect(screen.getByTestId("appearance-body")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Appearance" })).toBeTruthy();
    expect(screen.getByText("Tune the interface")).toBeTruthy();
    expect(screen.getAllByText("Plugins").length).toBeGreaterThan(0);
  });

  it("switches the body when a different pane is clicked", async () => {
    await loadPanes();
    testClient.render(<SettingsPage />);
    fireEvent.click(screen.getByText("Plugins"));
    expect(screen.getByTestId("plugins-body")).toBeTruthy();
    await waitFor(() => expect(screen.getAllByRole("tabpanel")).toHaveLength(1));
    expect(screen.getByRole("tabpanel").textContent).toContain("plugins body");
  });

  it("falls back to the first pane when no panes match the saved selection", async () => {
    await loadPanes();
    testClient.navigation.go({ settings: "removed" });
    testClient.render(<SettingsPage />);
    expect(screen.getByTestId("appearance-body")).toBeTruthy();
  });

  it("keeps the open pane and offers a way out when nothing matches", async () => {
    await loadPanes();
    testClient.render(<SettingsPage />);
    const search = screen.getByRole("searchbox");
    fireEvent.change(search, { target: { value: "zzzz" } });

    expect(screen.getByTestId("appearance-body")).toBeTruthy();
    expect(screen.getByRole("status").textContent).toContain("zzzz");
    const clear = within(screen.getByRole("status")).getByRole("button", {
      name: /clear search/i,
    });
    clear.focus();
    fireEvent.click(clear);
    expect(screen.getAllByRole("tab")).toHaveLength(2);
    expect(document.activeElement).toBe(search);
  });

  it.each([null, "removed"])(
    "keeps the default pane named while filtering with navigation target %s",
    async (target) => {
      await loadPanes();
      testClient.navigation.go({ settings: target });
      testClient.render(<SettingsPage />);
      const search = screen.getByRole("searchbox");
      fireEvent.change(search, { target: { value: "hot reload" } });
      expect(screen.queryByRole("tab", { name: "Appearance" })).toBeNull();
      await waitFor(() => expect(screen.getAllByRole("tabpanel")).toHaveLength(1));
      expect(screen.getByRole("tabpanel", { name: "Appearance" }).textContent).toContain(
        "appearance body",
      );
      expect(testClient.navigation.get().settings).toBe(target);
      fireEvent.keyDown(search, { key: "Enter" });
      expect(screen.getByTestId("plugins-body")).toBeTruthy();
      expect(testClient.navigation.get().settings).toBe("plugins");
    },
  );

  it("finds a pane by a field it contains, not only by its title", async () => {
    await loadPanes();
    testClient.render(<SettingsPage />);
    fireEvent.change(screen.getByRole("searchbox"), { target: { value: "hot reload" } });
    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual(["Plugins"]);
  });

  it("names a failed pane by its title and recovers it in place", async () => {
    let failing = true;
    await loadPluginsForTest(
      definePlugin({
        name: "test.settings.failing",
        setup: (ctx) => {
          ctx.contribute(SETTINGS_PANE, {
            id: "broken-look",
            label: "settings.pane.appearance",
            component: () => {
              if (failing) throw new Error("appearance exploded");
              return <div>recovered body</div>;
            },
          });
        },
      }),
    );
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    try {
      testClient.render(<SettingsPage />);
      const search = screen.getByRole("searchbox");
      fireEvent.change(search, { target: { value: "appear" } });
      expect(screen.getByRole("tab", { name: "Appearance" })).toBeTruthy();
      const panel = screen.getByRole("tabpanel", { name: "Appearance" });
      expect(within(panel).getByRole("heading", { name: "Appearance" })).toBeTruthy();
      expect(within(panel).getByText("appearance exploded")).toBeTruthy();
      const alert = within(panel).getByRole("alert");
      expect(within(alert).getByText("Appearance")).toBeTruthy();
      expect(within(alert).queryByText("settings:broken-look")).toBeNull();
      expect(usePluginErrorStore.getState().log).toContainEqual(
        expect.objectContaining({ plugin: "settings:broken-look", source: "render" }),
      );

      failing = false;
      const retry = within(panel).getByRole("button", { name: /retry/i });
      retry.focus();
      fireEvent.click(retry);

      expect(await within(panel).findByText("recovered body")).toBeTruthy();
      expect(within(panel).queryByText("appearance exploded")).toBeNull();
      expect(document.activeElement).toBe(panel);
      expect((search as HTMLInputElement).value).toBe("appear");
    } finally {
      consoleError.mockRestore();
    }
  });

  it("reveals the selected category below the rail after a resize without disturbing the edit", async () => {
    await loadRailPanes();
    const observers = stubResizeObservers();
    testClient.navigation.go({ settings: "mcp" });
    testClient.render(<SettingsPage />);
    const list = screen.getByRole("tablist");
    const frame = { height: 400 };
    mockRailGeometry(list, frame);
    const input = screen.getByRole<HTMLInputElement>("textbox", { name: "Description" });
    input.focus();
    input.setSelectionRange(3, 9);
    const panel = screen.getByRole("tabpanel", { name: "MCP servers" });
    panel.scrollTop = 40;

    observers.resize(list);
    expect(list.scrollTop).toBe(0);

    frame.height = 120;
    observers.resize(list);
    expect(list.scrollTop).toBe(72);
    observers.resize(list);
    expect(list.scrollTop).toBe(72);

    expect(screen.getByRole("tab", { name: "MCP servers" }).getAttribute("aria-selected")).toBe(
      "true",
    );
    expect(document.activeElement).toBe(input);
    expect(input.value).toBe("workspace tools draft");
    expect([input.selectionStart, input.selectionEnd]).toEqual([3, 9]);
    expect(panel.scrollTop).toBe(40);
    expect(window.scrollY).toBe(0);
    expect(document.documentElement.scrollTop).toBe(0);
  });

  it("reveals a category clipped above when its group resizes, leaving manual scrolling alone", async () => {
    await loadRailPanes();
    const observers = stubResizeObservers();
    testClient.navigation.go({ settings: "appearance" });
    testClient.render(<SettingsPage />);
    const list = screen.getByRole("tablist");
    mockRailGeometry(list, { height: 120 });

    list.scrollTop = 300;
    fireEvent.scroll(list);
    expect(list.scrollTop).toBe(300);

    const group = screen.getByRole("tab", { name: "Appearance" }).parentElement as HTMLElement;
    observers.resize(group);
    expect(list.scrollTop).toBe(40);
    expect(screen.getByRole("tabpanel", { name: "Appearance" }).textContent).toContain(
      "Appearance body",
    );
  });

  it("reveals the selected category when navigation changes without a resize", async () => {
    await loadRailPanes();
    stubResizeObservers();
    testClient.render(<SettingsPage />);
    const list = screen.getByRole("tablist");
    mockRailGeometry(list, { height: 120 });

    act(() => testClient.navigation.go({ settings: "plugins" }));
    expect(list.scrollTop).toBe(112);
    act(() => testClient.navigation.go({ settings: "providers" }));
    expect(list.scrollTop).toBe(80);
    act(() => testClient.navigation.go({ settings: "approvals" }));
    expect(list.scrollTop).toBe(80);
    expect(screen.getByRole("tab", { name: "Approvals" }).getAttribute("aria-selected")).toBe(
      "true",
    );
  });

  it("keeps the rail position when search hides the active category", async () => {
    await loadRailPanes();
    const observers = stubResizeObservers();
    testClient.navigation.go({ settings: "mcp" });
    testClient.render(<SettingsPage />);
    const list = screen.getByRole("tablist");
    mockRailGeometry(list, { height: 120 });
    list.scrollTop = 50;

    fireEvent.change(screen.getByRole("searchbox"), { target: { value: "hot reload" } });
    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual(["Plugins"]);
    observers.resize(list);
    expect(list.scrollTop).toBe(50);
    expect(screen.getByRole("tabpanel", { name: "MCP servers" })).toBeTruthy();
    expect(testClient.navigation.get().settings).toBe("mcp");
  });

  it("retires queued rail resize callbacks after Settings closes", async () => {
    await loadRailPanes();
    const observers = stubResizeObservers();
    testClient.navigation.go({ settings: "mcp" });
    testClient.render(<SettingsPage />);
    const list = screen.getByRole("tablist");
    mockRailGeometry(list, { height: 120 });
    const queued = observers.queuedFor(list);
    expect(queued.length).toBeGreaterThan(0);

    cleanup();
    act(() => queued.forEach((deliver) => deliver()));
    expect(list.scrollTop).toBe(0);
  });
});

import { beforeEach, describe, expect, it, vi } from "vitest";
import { useUiStore } from "./uiStore";

const ui = () => useUiStore.getState();

beforeEach(() => {
  useUiStore.setState({ sidebarCollapsed: false, sidebarCollapsedBy: null });
});

describe("who closed the drawer", () => {
  it("records a person's own collapse and reopen", () => {
    ui().toggleSidebar();
    expect(ui().sidebarCollapsed).toBe(true);
    expect(ui().sidebarCollapsedBy).toBe("manual");

    ui().toggleSidebar();
    expect(ui().sidebarCollapsed).toBe(false);
    expect(ui().sidebarCollapsedBy).toBeNull();
  });

  it("hands the measure straight back once the window can afford it again", () => {
    ui().setSidebarAutoCollapsed(true);
    expect(ui().sidebarCollapsed).toBe(true);
    expect(ui().sidebarCollapsedBy).toBe("auto");

    ui().setSidebarAutoCollapsed(false);
    expect(ui().sidebarCollapsed).toBe(false);
  });

  it("never reopens a drawer the person closed themselves", () => {
    ui().toggleSidebar();
    ui().setSidebarAutoCollapsed(true);
    expect(ui().sidebarCollapsedBy).toBe("manual");

    ui().setSidebarAutoCollapsed(false);
    expect(ui().sidebarCollapsed).toBe(true);
    expect(ui().sidebarCollapsedBy).toBe("manual");
  });

  it("leaves a person's reopen standing while the window still wants the measure", () => {
    ui().setSidebarAutoCollapsed(true);
    ui().toggleSidebar();
    expect(ui().sidebarCollapsed).toBe(false);
    expect(ui().sidebarCollapsedBy).toBeNull();
  });

  it("commits nothing on a resize tick that changes no answer", () => {
    const listener = vi.fn();
    const unsubscribe = useUiStore.subscribe(listener);

    ui().setSidebarAutoCollapsed(false);
    ui().setSidebarAutoCollapsed(false);
    expect(listener).not.toHaveBeenCalled();

    ui().setSidebarAutoCollapsed(true);
    expect(listener).toHaveBeenCalledTimes(1);

    ui().setSidebarAutoCollapsed(true);
    expect(listener).toHaveBeenCalledTimes(1);

    unsubscribe();
  });
});

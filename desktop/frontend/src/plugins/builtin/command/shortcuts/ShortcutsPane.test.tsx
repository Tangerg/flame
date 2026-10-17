import { act, fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { COMMAND, SHORTCUT, definePlugin, useShortcutOverrides } from "@/plugins/sdk";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
import { ShortcutsPane } from "./ShortcutsPane";

const runs = { open: vi.fn(), search: vi.fn() };

beforeEach(async () => {
  useShortcutOverrides.setState({ overrides: {}, recording: false });
  runs.open.mockReset();
  runs.search.mockReset();
  await loadPluginsForTest(
    definePlugin({
      name: "test.shortcuts",
      setup(ctx) {
        ctx.contribute(COMMAND, {
          id: "open",
          label: "Open thing",
          combo: "Mod+O",
          run: runs.open,
        });
        ctx.contribute(COMMAND, {
          id: "search",
          label: "Search thing",
          combo: "Mod+K",
          run: runs.search,
        });
        ctx.contribute(SHORTCUT, {
          key: "Mod+N",
          description: "New session",
          handler: () => undefined,
        });
      },
    }),
  );
});

function press(init: KeyboardEventInit) {
  act(() => {
    screen
      .getByRole("textbox", { name: /change the shortcut for/i })
      .dispatchEvent(new KeyboardEvent("keydown", { bubbles: true, ...init }));
  });
}

describe("ShortcutsPane", () => {
  it("records a new shortcut and remembers it as an override", () => {
    render(<ShortcutsPane />);
    fireEvent.click(screen.getByRole("button", { name: /change the shortcut for open thing/i }));
    expect(useShortcutOverrides.getState().recording).toBe(true);

    press({ key: "j", code: "KeyJ", ctrlKey: true, metaKey: true });
    expect(useShortcutOverrides.getState().overrides.open).toMatch(/j$/);
    expect(useShortcutOverrides.getState().recording).toBe(false);
  });

  it("explains a conflict and only takes the keys over when asked", () => {
    render(<ShortcutsPane />);
    fireEvent.click(screen.getByRole("button", { name: /change the shortcut for open thing/i }));
    press({ key: "k", code: "KeyK", ctrlKey: true, metaKey: true });

    expect(screen.getByRole("alert").textContent).toContain("Search thing");
    expect(useShortcutOverrides.getState().recording).toBe(true);
    expect(useShortcutOverrides.getState().overrides.open).toBeUndefined();

    fireEvent.click(screen.getByRole("button", { name: /use it here instead/i }));
    expect(useShortcutOverrides.getState().overrides.search).toBeNull();
    expect(useShortcutOverrides.getState().overrides.open).toMatch(/k$/);
    expect(useShortcutOverrides.getState().recording).toBe(false);
  });

  it("puts a changed shortcut back to its default", () => {
    useShortcutOverrides.setState({ overrides: { open: "mod+j" } });
    render(<ShortcutsPane />);
    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
    expect(useShortcutOverrides.getState().overrides).toEqual({});
  });

  it("cancels recording without admitting a shortcut override", () => {
    render(<ShortcutsPane />);
    fireEvent.click(screen.getByRole("button", { name: /change the shortcut for open thing/i }));
    press({ key: "Control", code: "ControlLeft", ctrlKey: true });
    expect(useShortcutOverrides.getState().recording).toBe(true);
    expect(useShortcutOverrides.getState().overrides).toEqual({});
    fireEvent.keyDown(screen.getByRole("button", { name: "Cancel" }), {
      key: "Escape",
      code: "Escape",
    });
    expect(useShortcutOverrides.getState().recording).toBe(false);
    expect(useShortcutOverrides.getState().overrides).toEqual({});
  });

  it("explains a fixed shortcut conflict without offering to replace it", () => {
    render(<ShortcutsPane />);
    fireEvent.click(screen.getByRole("button", { name: /change the shortcut for open thing/i }));
    press({ key: "n", code: "KeyN", ctrlKey: true, metaKey: true });
    expect(screen.getByRole("alert").textContent).toContain("New session");
    expect(useShortcutOverrides.getState().recording).toBe(true);
    expect(screen.queryByRole("button", { name: /use it here instead/i })).toBeNull();
    expect(useShortcutOverrides.getState().overrides).toEqual({});
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(useShortcutOverrides.getState().recording).toBe(false);
    expect(useShortcutOverrides.getState().overrides).toEqual({});
  });

  it("releases recording when the pane unmounts", () => {
    const { unmount } = render(<ShortcutsPane />);
    fireEvent.click(screen.getByRole("button", { name: /change the shortcut for open thing/i }));
    expect(useShortcutOverrides.getState().recording).toBe(true);
    unmount();
    expect(useShortcutOverrides.getState().recording).toBe(false);
    window.dispatchEvent(
      new KeyboardEvent("keydown", { key: "j", code: "KeyJ", ctrlKey: true, metaKey: true }),
    );
    expect(useShortcutOverrides.getState().overrides).toEqual({});
  });

  it("captures keys only from the active recorder", () => {
    render(<ShortcutsPane />);
    fireEvent.click(screen.getByRole("button", { name: /change the shortcut for open thing/i }));
    window.dispatchEvent(
      new KeyboardEvent("keydown", { key: "j", code: "KeyJ", ctrlKey: true, metaKey: true }),
    );
    expect(useShortcutOverrides.getState().overrides).toEqual({});
    expect(useShortcutOverrides.getState().recording).toBe(true);
    expect(screen.getByRole<HTMLInputElement>("searchbox").disabled).toBe(true);
    expect(
      screen.getByRole<HTMLButtonElement>("button", {
        name: /change the shortcut for search thing/i,
      }).disabled,
    ).toBe(true);
    press({ key: "j", code: "KeyJ", ctrlKey: true, metaKey: true });
    expect(useShortcutOverrides.getState().overrides.open).toMatch(/j$/);
    expect(screen.getByRole<HTMLInputElement>("searchbox").disabled).toBe(false);
  });

  it("allows bare Enter in the recorder while Tab remains navigation", () => {
    render(<ShortcutsPane />);
    fireEvent.click(screen.getByRole("button", { name: /change the shortcut for open thing/i }));
    press({ key: "Tab", code: "Tab" });
    expect(useShortcutOverrides.getState().recording).toBe(true);
    expect(useShortcutOverrides.getState().overrides).toEqual({});
    press({ key: "Enter", code: "Enter" });
    expect(useShortcutOverrides.getState().overrides.open).toBe("enter");
    expect(useShortcutOverrides.getState().recording).toBe(false);
  });
});

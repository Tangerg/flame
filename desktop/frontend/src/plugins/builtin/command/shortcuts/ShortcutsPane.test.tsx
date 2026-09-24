import { act, fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { COMMAND, definePlugin, useShortcutOverrides } from "@/plugins/sdk";
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
      },
    }),
  );
});

function press(init: KeyboardEventInit) {
  act(() => {
    window.dispatchEvent(new KeyboardEvent("keydown", { bubbles: true, ...init }));
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
    expect(useShortcutOverrides.getState().overrides.open).toBeUndefined();

    fireEvent.click(screen.getByRole("button", { name: /use it here instead/i }));
    expect(useShortcutOverrides.getState().overrides.search).toBeNull();
    expect(useShortcutOverrides.getState().overrides.open).toMatch(/k$/);
  });

  it("puts a changed shortcut back to its default", () => {
    useShortcutOverrides.setState({ overrides: { open: "mod+j" } });
    render(<ShortcutsPane />);
    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
    expect(useShortcutOverrides.getState().overrides).toEqual({});
  });
});

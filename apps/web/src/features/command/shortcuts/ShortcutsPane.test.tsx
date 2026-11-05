import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, fireEvent, screen } from "@testing-library/react";
import { ContentVisibility } from "@flame/ui";
import { asyncDisposeSymbol } from "dougong";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { COMMAND, SHORTCUT } from "@/features/command/public/extensionPoints";
import { definePlugin } from "@/extensions/definePlugin";
import { useShortcutOverrides } from "@/features/command/public/shortcutOverrides";
import { comboGlyph, splitCombo } from "@/lib/combo";
import { contributeForTest, loadPluginsForTest } from "@/test/kernel";
import { ShortcutsPane } from "@/features/command/shortcuts/ShortcutsPane";
import { ShortcutsProvider } from "@/features/command/ShortcutsProvider";
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

function pressGlobally(init: KeyboardEventInit) {
  act(() => {
    window.dispatchEvent(new KeyboardEvent("keydown", init));
  });
}

function RetainedPane({ visible }: { visible: boolean }) {
  return (
    <>
      <ShortcutsProvider />
      <button type="button">Outside</button>
      <ContentVisibility visible={visible}>
        <ContentVisibility visible>
          <ShortcutsPane />
        </ContentVisibility>
      </ContentVisibility>
    </>
  );
}

describe("ShortcutsPane", () => {
  it("records a new shortcut and remembers it as an override", () => {
    testClient.render(<ShortcutsPane />);
    fireEvent.click(screen.getByRole("button", { name: /change the shortcut for open thing/i }));
    expect(useShortcutOverrides.getState().recording).toBe(true);

    press({ key: "j", code: "KeyJ", ctrlKey: true, metaKey: true });
    expect(useShortcutOverrides.getState().overrides.open).toMatch(/j$/);
    expect(useShortcutOverrides.getState().recording).toBe(false);
  });

  it("re-evaluates a fresh combo after a conflict instead of freezing", () => {
    testClient.render(<ShortcutsPane />);
    fireEvent.click(screen.getByRole("button", { name: /change the shortcut for open thing/i }));
    press({ key: "k", code: "KeyK", ctrlKey: true, metaKey: true });
    expect(screen.getByRole("alert")).toBeTruthy();
    expect(useShortcutOverrides.getState().overrides.open).toBeUndefined();

    press({ key: "j", code: "KeyJ", ctrlKey: true, metaKey: true });
    expect(useShortcutOverrides.getState().overrides.open).toMatch(/j$/);
    expect(useShortcutOverrides.getState().recording).toBe(false);
  });

  it("explains a conflict and only takes the keys over when asked", () => {
    testClient.render(<ShortcutsPane />);
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
    testClient.render(<ShortcutsPane />);
    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
    expect(useShortcutOverrides.getState().overrides).toEqual({});
  });

  it("cancels recording without admitting a shortcut override", () => {
    testClient.render(<ShortcutsPane />);
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
    testClient.render(<ShortcutsPane />);
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
    const { unmount } = testClient.render(<ShortcutsPane />);
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
    testClient.render(<ShortcutsPane />);
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

  it("records Space as a binding that dispatches and resets to the original", () => {
    const space = { key: " ", code: "Space" };
    const view = testClient.render(
      <>
        <ShortcutsProvider />
        <ShortcutsPane />
      </>,
    );
    const activate = (init: KeyboardEventInit) =>
      act(() => {
        window.dispatchEvent(new KeyboardEvent("keydown", init));
      });
    fireEvent.click(screen.getByRole("button", { name: /change the shortcut for open thing/i }));

    press(space);
    expect(useShortcutOverrides.getState().overrides.open).toBe("space");
    expect(useShortcutOverrides.getState().recording).toBe(false);
    expect(runs.open).not.toHaveBeenCalled();
    expect(screen.getByText("Space")).toBeTruthy();

    activate(space);
    expect(runs.open).toHaveBeenCalledOnce();

    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
    expect(useShortcutOverrides.getState().overrides).toEqual({});
    expect(screen.queryByText("Space")).toBeNull();
    activate(space);
    expect(runs.open).toHaveBeenCalledOnce();
    activate({ key: "o", code: "KeyO", ctrlKey: true });
    expect(runs.open).toHaveBeenCalledTimes(2);
    view.unmount();
  });

  it("allows bare Enter in the recorder while Tab remains navigation", () => {
    testClient.render(<ShortcutsPane />);
    fireEvent.click(screen.getByRole("button", { name: /change the shortcut for open thing/i }));
    press({ key: "Tab", code: "Tab" });
    expect(useShortcutOverrides.getState().recording).toBe(true);
    expect(useShortcutOverrides.getState().overrides).toEqual({});
    press({ key: "Enter", code: "Enter" });
    expect(useShortcutOverrides.getState().overrides.open).toBe("enter");
    expect(useShortcutOverrides.getState().recording).toBe(false);
  });

  it("retires a recording hidden by an inherited scope without applying it", () => {
    useShortcutOverrides.setState({ overrides: { search: "mod+j" } });
    const view = testClient.render(<RetainedPane visible />);
    fireEvent.change(screen.getByRole("searchbox"), { target: { value: "thing" } });
    fireEvent.click(screen.getByRole("button", { name: /change the shortcut for open thing/i }));
    press({ key: "n", code: "KeyN", ctrlKey: true, metaKey: true });
    expect(screen.getByRole("alert").textContent).toContain("New session");

    const outside = screen.getByRole("button", { name: "Outside" });
    act(() => outside.focus());
    view.rerender(<RetainedPane visible={false} />);
    expect(document.activeElement).toBe(outside);
    expect(useShortcutOverrides.getState().recording).toBe(false);
    expect(useShortcutOverrides.getState().overrides).toEqual({ search: "mod+j" });
    expect(screen.queryByRole("alert")).toBeNull();
    pressGlobally({ key: "o", code: "KeyO", ctrlKey: true });
    expect(runs.open).toHaveBeenCalledOnce();

    view.rerender(<RetainedPane visible />);
    expect(screen.queryByRole("textbox", { name: /change the shortcut for/i })).toBeNull();
    expect(useShortcutOverrides.getState().recording).toBe(false);
    expect(screen.getByRole<HTMLInputElement>("searchbox").value).toBe("thing");
    expect(screen.getByRole<HTMLInputElement>("searchbox").disabled).toBe(false);
    expect(document.activeElement).toBe(outside);
    view.unmount();
  });

  it("retires a capture when an updated label leaves its retained query", async () => {
    let rename!: (label: string) => void;
    const run = vi.fn();
    await contributeForTest((ctx) => {
      const command = ctx.contribute(COMMAND, { id: "effect", label: "Effect thing", run });
      rename = (label) => command.update({ id: "effect", label, run });
    });
    const view = testClient.render(<RetainedPane visible />);
    const filter = screen.getByRole<HTMLInputElement>("searchbox");
    fireEvent.change(filter, { target: { value: "Effect" } });
    fireEvent.click(screen.getByRole("button", { name: /change the shortcut for effect thing/i }));
    const outside = screen.getByRole("button", { name: "Outside" });
    act(() => outside.focus());

    act(() => rename("Different label"));
    expect(useShortcutOverrides.getState().recording).toBe(false);
    expect(useShortcutOverrides.getState().overrides).toEqual({});
    expect(filter.value).toBe("Effect");
    expect(filter.disabled).toBe(false);
    expect(document.activeElement).toBe(outside);
    pressGlobally({ key: "o", code: "KeyO", ctrlKey: true });
    expect(runs.open).toHaveBeenCalledOnce();

    act(() => rename("Effect thing"));
    expect(screen.queryByRole("textbox", { name: /change the shortcut for/i })).toBeNull();
    expect(
      screen.getByRole<HTMLButtonElement>("button", {
        name: /change the shortcut for effect thing/i,
      }).disabled,
    ).toBe(false);
    expect(document.activeElement).toBe(outside);
    view.unmount();
  });

  it("filters by labels, stored combos and the displayed keys", async () => {
    await contributeForTest((ctx) => {
      ctx.contribute(SHORTCUT, {
        key: "ArrowUp",
        description: "Previous item",
        handler: () => undefined,
      });
    });
    testClient.render(<ShortcutsPane />);
    const labels = ["New session", "Open thing", "Previous item", "Search thing"];
    const shown = () => labels.filter((label) => screen.queryByText(label) !== null);
    const filter = screen.getByRole<HTMLInputElement>("searchbox");
    const cases: [string, string[]][] = [
      [comboGlyph("Mod"), ["New session", "Open thing", "Search thing"]],
      [`  ${comboGlyph("Mod+K").toUpperCase()}  `, ["Search thing"]],
      [splitCombo("Mod+K").join(" ").toLowerCase(), ["Search thing"]],
      ["↑", ["Previous item"]],
      ["search", ["Search thing"]],
      ["mod+n", ["New session"]],
    ];
    for (const [query, expected] of cases) {
      fireEvent.change(filter, { target: { value: query } });
      expect(shown()).toEqual(expected);
    }

    fireEvent.change(filter, { target: { value: "zzz" } });
    expect(shown()).toEqual([]);
    expect(screen.getByText("No shortcuts match")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /clear/i }));
    expect(filter.value).toBe("");
    expect(shown()).toEqual(labels);
  });

  it("filters displayed keys by the effective combo through rebind and reset", () => {
    testClient.render(<ShortcutsPane />);
    const filter = screen.getByRole<HTMLInputElement>("searchbox");
    const defaultKeys = splitCombo("Mod+O").join(" ");
    fireEvent.change(filter, { target: { value: defaultKeys } });
    fireEvent.click(screen.getByRole("button", { name: /change the shortcut for open thing/i }));
    press({ key: "j", code: "KeyJ", ctrlKey: true, metaKey: true });

    const rebound = useShortcutOverrides.getState().overrides.open ?? "";
    expect(rebound).toMatch(/j$/);
    expect(screen.queryByText("Open thing")).toBeNull();
    expect(filter.value).toBe(defaultKeys);
    expect(document.activeElement).toBe(filter);

    fireEvent.change(filter, { target: { value: comboGlyph(rebound) } });
    expect(screen.getByText("Open thing")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
    expect(useShortcutOverrides.getState().overrides).toEqual({});
    expect(screen.queryByText("Open thing")).toBeNull();
    expect(filter.value).toBe(comboGlyph(rebound));
    expect(document.activeElement).toBe(filter);

    fireEvent.change(filter, { target: { value: defaultKeys } });
    expect(screen.getByText("Open thing")).toBeTruthy();
  });

  it("keeps a capture through row updates and retires it with its command source", async () => {
    let renameOther!: () => void;
    let renameEffect!: () => void;
    let withdraw!: () => Promise<void>;
    await contributeForTest((ctx) => {
      const source = ctx.lifetime("source");
      const effect = source.contribute(COMMAND, {
        id: "effect",
        label: "Effect thing",
        run: vi.fn(),
      });
      const other = ctx.contribute(COMMAND, { id: "other", label: "Other thing", run: vi.fn() });
      renameEffect = () => effect.update({ id: "effect", label: "Effect renamed", run: vi.fn() });
      renameOther = () => other.update({ id: "other", label: "Other renamed", run: vi.fn() });
      withdraw = () => source[asyncDisposeSymbol]();
    });
    const view = testClient.render(<RetainedPane visible />);
    fireEvent.click(screen.getByRole("button", { name: /change the shortcut for effect thing/i }));
    press({ key: "k", code: "KeyK", ctrlKey: true, metaKey: true });
    expect(screen.getByRole("alert").textContent).toContain("Search thing");

    act(() => {
      renameOther();
      renameEffect();
    });
    expect(screen.getByRole("alert").textContent).toContain("Search thing");
    expect(
      screen.getByRole("textbox", { name: /change the shortcut for effect renamed/i }),
    ).toBeTruthy();
    expect(useShortcutOverrides.getState().recording).toBe(true);

    await act(withdraw);
    expect(useShortcutOverrides.getState().recording).toBe(false);
    expect(useShortcutOverrides.getState().overrides).toEqual({});
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.queryByRole("button", { name: "Cancel" })).toBeNull();
    expect(
      screen.getByRole<HTMLButtonElement>("button", { name: /change the shortcut for open thing/i })
        .disabled,
    ).toBe(false);
    pressGlobally({ key: "o", code: "KeyO", ctrlKey: true });
    expect(runs.open).toHaveBeenCalledOnce();
    view.unmount();
  });
});

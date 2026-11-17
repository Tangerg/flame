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
import { beforeEach, describe, expect, it, vi } from "vitest";
import { COMMAND, SHORTCUT } from "@/features/command/public/extensionPoints";
import { definePlugin } from "@/extensions/definePlugin";
import { useShortcutOverrides } from "@/features/command/public/shortcutOverrides";
import { normalizeHotkey } from "@tanstack/hotkeys";
import { loadPluginsForTest } from "@/test/kernel";
import { ShortcutsPane } from "@/features/command/shortcuts/ShortcutsPane";
import { ShortcutsProvider } from "@/features/command/ShortcutsProvider";
const runs = { open: vi.fn(), search: vi.fn() };

beforeEach(async () => {
  useShortcutOverrides.setState({ overrides: {} });
  runs.open.mockReset();
  runs.search.mockReset();
  await loadPluginsForTest(
    definePlugin({
      name: "test.shortcuts",
      setup(ctx) {
        ctx.contributions.add(COMMAND, {
          id: "open",
          label: "Open thing",
          combo: "Mod+O",
          run: runs.open,
        });
        ctx.contributions.add(COMMAND, {
          id: "search",
          label: "Search thing",
          combo: "Mod+K",
          run: runs.search,
        });
        ctx.contributions.add(SHORTCUT, {
          key: "Mod+N",
          description: "New session",
          handler: () => undefined,
        });
      },
    }),
  );
});

function press(key: string) {
  act(() => {
    document.dispatchEvent(
      new KeyboardEvent("keydown", {
        key,
        code: `Key${key.toUpperCase()}`,
        ctrlKey: true,
        bubbles: true,
        cancelable: true,
      }),
    );
  });
}
describe("shortcut recording", () => {
  it("lets the library record a canonical override and release recording", () => {
    testClient.render(<ShortcutsPane />);
    fireEvent.click(screen.getByRole("button", { name: /change the shortcut for open thing/i }));
    press("j");
    expect(useShortcutOverrides.getState().overrides.open).toBe(normalizeHotkey("Control+J"));
    expect(screen.queryByRole("textbox", { name: /change the shortcut for/i })).toBeNull();
  });
  it("withdraws a hidden recorder and leaves command dispatch available", () => {
    const view = testClient.render(
      <>
        <ShortcutsProvider />
        <ContentVisibility visible>
          <ShortcutsPane />
        </ContentVisibility>
      </>,
    );
    fireEvent.click(screen.getByRole("button", { name: /change the shortcut for open thing/i }));
    view.rerender(
      <>
        <ShortcutsProvider />
        <ContentVisibility visible={false}>
          <ShortcutsPane />
        </ContentVisibility>
      </>,
    );
    press("j");
    expect(useShortcutOverrides.getState().overrides.open).toBeUndefined();
  });
});

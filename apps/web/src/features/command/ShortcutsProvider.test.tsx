import { beforeEach, onTestFinished, expect, it, vi } from "vitest";
import { fireEvent, screen } from "@testing-library/react";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { definePlugin } from "@/extensions/definePlugin";
import { COMMAND } from "@/features/command/public/extensionPoints";
import { loadPluginsForTest } from "@/test/kernel";
import { useShortcutOverrides } from "@/features/command/public/shortcutOverrides";
import { ShortcutsProvider } from "./ShortcutsProvider";
let client: TestClient;
beforeEach(() => {
  client = createTestClient();
  onTestFinished(() => client.dispose());
  useShortcutOverrides.setState({ overrides: {} });
});
it("dispatches a command only after the input owner and IME have declined the event", async () => {
  const run = vi.fn();
  await loadPluginsForTest(
    definePlugin({
      name: "test.shortcuts",
      setup(ctx) {
        ctx.contributions.add(COMMAND, { id: "test.act", label: "Act", combo: "Control+K", run });
      },
    }),
  );
  client.render(
    <>
      <ShortcutsProvider />
      <input aria-label="Draft" />
    </>,
  );
  const input = screen.getByRole("textbox");
  input.focus();
  fireEvent.keyDown(input, { key: "k", code: "KeyK", ctrlKey: true, isComposing: true });
  expect(run).not.toHaveBeenCalled();
  const consumed = new KeyboardEvent("keydown", {
    key: "k",
    code: "KeyK",
    ctrlKey: true,
    bubbles: true,
    cancelable: true,
  });
  consumed.preventDefault();
  input.dispatchEvent(consumed);
  expect(run).not.toHaveBeenCalled();
  fireEvent.keyDown(input, { key: "k", code: "KeyK", ctrlKey: true });
  expect(run).toHaveBeenCalledOnce();
});

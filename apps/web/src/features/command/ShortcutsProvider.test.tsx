import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});

import { useRef } from "react";
import { describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import { selectAllInReader } from "@flame/ui";
import { definePlugin } from "@/extensions/definePlugin";
import { useShortcutOverrides } from "@/features/command/public/shortcutOverrides";
import { COMMAND, SHORTCUT } from "@/features/command/public/extensionPoints";
import { loadPluginsForTest } from "@/test/kernel";
import { ShortcutsProvider } from "@/features/command/ShortcutsProvider";
import { ScrollArea } from "@/ui/atoms/scroll-area";
const press = (init: Partial<KeyboardEventInit> = {}) =>
  window.dispatchEvent(
    new KeyboardEvent("keydown", { key: "k", code: "KeyK", altKey: true, ...init }),
  );

describe("shortcuts provider", () => {
  it("binds a command's combo without anything else listing the command", async () => {
    const run = vi.fn();
    await loadPluginsForTest(
      definePlugin({
        name: "test.command.combo",
        setup: (ctx) => {
          ctx.contributions.add(COMMAND, { id: "test.act", label: "Act", combo: "Alt+K", run });
        },
      }),
    );
    testClient.render(<ShortcutsProvider />);

    press();

    expect(run).toHaveBeenCalledOnce();
  });

  it("lets an explicit shortcut take a key a command already carries", async () => {
    const command = vi.fn();
    const shortcut = vi.fn();
    await loadPluginsForTest(
      definePlugin({
        name: "test.command.contested",
        setup: (ctx) => {
          ctx.contributions.add(COMMAND, {
            id: "test.act",
            label: "Act",
            combo: "Alt+K",
            run: command,
          });
          ctx.contributions.add(SHORTCUT, {
            key: "Alt+K",
            description: "shortcut.test",
            allowInInputs: true,
            handler: shortcut,
          });
        },
      }),
    );
    testClient.render(<ShortcutsProvider />);

    press();

    expect(shortcut).toHaveBeenCalledOnce();
    expect(command).not.toHaveBeenCalled();
  });

  it("keeps a bare-key command from firing inside a text field but runs a modified one", async () => {
    const bare = vi.fn();
    const modified = vi.fn();
    await loadPluginsForTest(
      definePlugin({
        name: "test.command.inputsafe",
        setup: (ctx) => {
          ctx.contributions.add(COMMAND, { id: "test.bare", label: "Bare", combo: "j", run: bare });
          ctx.contributions.add(COMMAND, {
            id: "test.mod",
            label: "Mod",
            combo: "Alt+K",
            run: modified,
          });
        },
      }),
    );
    const view = testClient.render(<ShortcutsProvider />);
    const input = document.createElement("input");
    document.body.appendChild(input);

    input.dispatchEvent(new KeyboardEvent("keydown", { key: "j", code: "KeyJ", bubbles: true }));
    expect(bare).not.toHaveBeenCalled();

    press({ key: "j", code: "KeyJ", altKey: false });
    expect(bare).toHaveBeenCalledOnce();

    input.dispatchEvent(
      new KeyboardEvent("keydown", { key: "k", code: "KeyK", altKey: true, bubbles: true }),
    );
    expect(modified).toHaveBeenCalledOnce();

    input.remove();
    view.unmount();
  });

  it("follows a user's rebinding and does not run anything while one is being recorded", async () => {
    const run = vi.fn();
    useShortcutOverrides.setState({ overrides: { "test.rebound": "alt+j" }, recording: false });
    await loadPluginsForTest(
      definePlugin({
        name: "test.command.rebound",
        setup: (ctx) => {
          ctx.contributions.add(COMMAND, { id: "test.rebound", label: "Act", combo: "Alt+K", run });
        },
      }),
    );
    const view = testClient.render(<ShortcutsProvider />);

    press();
    expect(run).not.toHaveBeenCalled();
    press({ key: "j", code: "KeyJ" });
    expect(run).toHaveBeenCalledOnce();

    useShortcutOverrides.setState({ recording: true });
    press({ key: "j", code: "KeyJ" });
    expect(run).toHaveBeenCalledOnce();

    useShortcutOverrides.setState({ overrides: {}, recording: false });
    view.unmount();
  });

  it.each([
    [
      "command",
      (run: () => void) =>
        definePlugin({
          name: "test.command.consumed",
          setup: (ctx) => {
            ctx.contributions.add(COMMAND, {
              id: "test.focus",
              label: "Focus",
              combo: "Ctrl+A",
              run,
            });
          },
        }),
    ],
    [
      "shortcut",
      (run: () => void) =>
        definePlugin({
          name: "test.shortcut.consumed",
          setup: (ctx) => {
            ctx.contributions.add(SHORTCUT, { key: "Ctrl+A", handler: run });
          },
        }),
    ],
  ])("leaves a %s's combo to a reader that already selected all", async (_, plugin) => {
    const run = vi.fn();
    await loadPluginsForTest(plugin(run));
    function Reader() {
      const content = useRef<HTMLPreElement>(null);
      return (
        <>
          <ScrollArea
            role="region"
            aria-label="Tool output"
            tabIndex={0}
            onKeyDown={(event) => selectAllInReader(event, content.current!)}
          >
            <span>go test ./...</span>
            <pre ref={content}>ok flame</pre>
          </ScrollArea>
          <button type="button">Composer</button>
        </>
      );
    }
    const view = testClient.render(
      <>
        <ShortcutsProvider />
        <Reader />
      </>,
    );
    const selectAll = (target: HTMLElement) => {
      target.focus();
      return target.dispatchEvent(
        new KeyboardEvent("keydown", {
          key: "a",
          code: "KeyA",
          ctrlKey: true,
          bubbles: true,
          cancelable: true,
        }),
      );
    };
    const reader = screen.getByRole("region", { name: "Tool output" });

    expect(selectAll(reader)).toBe(false);
    expect(document.getSelection()?.toString()).toBe("ok flame");
    expect(document.activeElement).toBe(reader);
    expect(run).not.toHaveBeenCalled();

    selectAll(screen.getByRole("button", { name: "Composer" }));
    expect(run).toHaveBeenCalledOnce();

    document.getSelection()?.removeAllRanges();
    view.unmount();
  });
});

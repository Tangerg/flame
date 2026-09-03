import { render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { definePlugin } from "../sdk";
import { COMMAND, SHORTCUT } from "@/plugins/sdk/kernelPoints";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
import { ShortcutsProvider } from "./ShortcutsProvider";

// Alt rather than Mod: `$mod` resolves to Meta or Control by platform, and the point here is
// the projection, not the modifier table.
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
        setup: (ctx) =>
          ctx.contribute(COMMAND, { id: "test.act", label: "Act", combo: "Alt+K", run }),
      }),
    );
    render(<ShortcutsProvider />);

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
          ctx.contribute(COMMAND, {
            id: "test.act",
            label: "Act",
            combo: "Alt+K",
            run: command,
          });
          ctx.contribute(SHORTCUT, {
            key: "Alt+K",
            description: "shortcut.test",
            allowInInputs: true,
            handler: shortcut,
          });
        },
      }),
    );
    render(<ShortcutsProvider />);

    press();

    expect(shortcut).toHaveBeenCalledOnce();
    expect(command).not.toHaveBeenCalled();
  });
});

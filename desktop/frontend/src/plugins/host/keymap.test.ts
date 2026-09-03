import type { CommandSpec } from "@/plugins/sdk";
import { describe, expect, it, vi } from "vitest";
import { commandShortcuts } from "./keymap";

const command = (patch: Partial<CommandSpec> & Pick<CommandSpec, "id">): CommandSpec => ({
  label: patch.id,
  run: () => undefined,
  ...patch,
});

describe("commandShortcuts", () => {
  it("binds every command that declares a combo, and only those", () => {
    const shortcuts = commandShortcuts([
      command({ id: "chat.new", label: "New chat", combo: "Mod+N" }),
      command({ id: "chat.rename" }),
    ]);

    expect(shortcuts).toHaveLength(1);
    expect(shortcuts[0]).toMatchObject({
      key: "Mod+N",
      description: "New chat",
      allowInInputs: true,
    });
  });

  it("takes over the key from the browser and runs the command", () => {
    const run = vi.fn();
    const shortcuts = commandShortcuts([command({ id: "chat.new", combo: "Mod+N", run })]);
    const event = { preventDefault: vi.fn() } as unknown as KeyboardEvent;

    shortcuts[0]?.handler(event);

    expect(event.preventDefault).toHaveBeenCalledOnce();
    expect(run).toHaveBeenCalledOnce();
  });
});

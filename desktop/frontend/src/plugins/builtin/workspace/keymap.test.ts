import { describe, expect, it, vi } from "vitest";
import { workspaceEscapeShortcut } from "./keymap";

describe("workspaceEscapeShortcut", () => {
  it("closes the workspace view", () => {
    const closeActiveView = vi.fn(() => true);
    const shortcut = workspaceEscapeShortcut(closeActiveView);

    expect(shortcut).toMatchObject({
      key: "Escape",
      description: "shortcut.closeWorkspaceView",
      allowInInputs: false,
    });

    shortcut.handler(new KeyboardEvent("keydown"));
    expect(closeActiveView).toHaveBeenCalledOnce();
  });
});

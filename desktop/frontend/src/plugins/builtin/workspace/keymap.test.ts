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

    // The palette used to own Escape while it was open, so this went through a
    // guard that asked first. One meaning now, so the handler just closes.
    shortcut.handler(new KeyboardEvent("keydown"));
    expect(closeActiveView).toHaveBeenCalledOnce();
  });
});

import type { ShortcutSpec } from "@/plugins/sdk";
import { definePlugin } from "@/plugins/sdk";
import { SHORTCUT } from "@/plugins/sdk/kernelPoints";
import { closeActiveWorkspaceView } from "./public/navigation";

/** Escape has one application meaning: close the active workspace view. */
export function workspaceEscapeShortcut(closeActiveView: () => boolean): ShortcutSpec {
  return {
    key: "Escape",
    description: "shortcut.closeWorkspaceView",
    allowInInputs: false,
    handler: () => {
      closeActiveView();
    },
  };
}

export const workspaceKeymap = definePlugin({
  name: "flame.builtin.workspace.keymap",
  setup(ctx) {
    ctx.contribute(SHORTCUT, workspaceEscapeShortcut(closeActiveWorkspaceView));
  },
});

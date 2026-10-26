import { type WorkspaceAccess } from "@/features/workspace/public/workspace";
import { type ShortcutSpec } from "@/features/command/public/commands";
import { definePlugin } from "@/extensions/definePlugin";
import { SHORTCUT } from "@/features/command/public/extensionPoints";
import { closeActiveWorkspaceView } from "@/features/workspace/public/navigation";
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

export const workspaceKeymap = (workspaceAccess: WorkspaceAccess) =>
  definePlugin({
    name: "flame.builtin.workspace.keymap",
    setup(ctx) {
      ctx.contribute(
        SHORTCUT,
        workspaceEscapeShortcut(closeActiveWorkspaceView.bind(null, workspaceAccess)),
      );
    },
  });

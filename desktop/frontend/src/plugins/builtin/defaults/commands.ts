import { toggleThemeScheme } from "@/plugins/builtin/theme/public/scheme";
import {
  closeActiveAgentSession,
  stepActiveAgentSession,
} from "@/plugins/builtin/agent/public/session";
import {
  closeActiveWorkspaceDockView,
  closeActiveWorkspaceView,
  toggleWorkspaceDock,
  toggleWorkspaceSidebar,
} from "@/plugins/builtin/workspace/public/navigation";
import { COMMAND, definePlugin } from "@/plugins/sdk";
import { focusComposer } from "@/plugins/builtin/chat/composer/public/focus";

import { navigator } from "@/lib/navigation";
import { defaultStaticCommands } from "./application/defaultContributions";
import { createNewSession } from "@/plugins/builtin/navigation/public/workIndex";

function closeFocusedSurface(): void {
  if (closeActiveWorkspaceView()) return;
  if (closeActiveWorkspaceDockView()) return;
  closeActiveAgentSession();
}

export const defaultCommands = definePlugin({
  name: "flame.builtin.default-commands",
  setup(ctx) {
    for (const command of defaultStaticCommands({
      toggleSidebar: toggleWorkspaceSidebar,
      toggleDock: toggleWorkspaceDock,
      toggleTheme: toggleThemeScheme,
      newChat: createNewSession,
      closeFocused: closeFocusedSurface,
      focusComposer: () => focusComposer(),
      historyBack: () => navigator().back(),
      historyForward: () => navigator().forward(),
      previousSession: () => stepActiveAgentSession(-1),
      nextSession: () => stepActiveAgentSession(1),
    })) {
      ctx.contribute(COMMAND, command);
    }
  },
});

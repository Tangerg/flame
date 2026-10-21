import { type CommandSpec } from "@/features/command/public/commands";
import { toggleThemeScheme } from "@/features/theme/public/scheme";
import { closeActiveAgentSession } from "@/features/agent/public/session";
import { stepActiveAgentSession } from "@/features/agent/public/session";
import {
  closeActiveWorkspaceDockView,
  closeActiveWorkspaceView,
  toggleWorkspaceDock,
  toggleWorkspaceSidebar,
} from "@/features/workspace/public/navigation";
import { COMMAND } from "@/features/command/public/extensionPoints";
import { definePlugin } from "@/extensions/definePlugin";
import { navigator } from "@/lib/navigation";
import { createNewSession } from "@/features/navigation/public/workIndex";
type CommandRun = CommandSpec["run"];

export interface DefaultCommandRuns {
  toggleSidebar: CommandRun;
  toggleDock: CommandRun;
  toggleTheme: CommandRun;
  newChat: CommandRun;
  closeFocused: CommandRun;
  historyBack: CommandRun;
  historyForward: CommandRun;
  previousSession: CommandRun;
  nextSession: CommandRun;
}

export function defaultStaticCommands(runs: DefaultCommandRuns): CommandSpec[] {
  return [
    {
      id: "view.toggle-sidebar",
      label: "command.toggleSidebar",
      combo: "Mod+B",
      run: runs.toggleSidebar,
    },
    {
      id: "view.toggle-dock",
      label: "command.toggleDock",
      combo: "Mod+Shift+B",
      run: runs.toggleDock,
    },
    {
      id: "settings.toggle-theme",
      label: "command.toggleTheme",
      combo: "Mod+Shift+L",
      run: runs.toggleTheme,
    },
    {
      id: "chat.new",
      label: "command.newChat",
      combo: "Mod+N",
      run: runs.newChat,
    },
    {
      id: "workspace.close-focused",
      label: "command.closeFocused",
      combo: "Mod+W",
      run: runs.closeFocused,
    },
    {
      id: "history.back",
      label: "command.historyBack",
      combo: "Mod+[",
      run: runs.historyBack,
    },
    {
      id: "history.forward",
      label: "command.historyForward",
      combo: "Mod+]",
      run: runs.historyForward,
    },
    {
      id: "session.previous",
      label: "command.previousSession",
      combo: "Mod+Shift+[",
      run: runs.previousSession,
    },
    {
      id: "session.next",
      label: "command.nextSession",
      combo: "Mod+Shift+]",
      run: runs.nextSession,
    },
  ];
}

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
      historyBack: () => navigator().back(),
      historyForward: () => navigator().forward(),
      previousSession: () => stepActiveAgentSession(-1),
      nextSession: () => stepActiveAgentSession(1),
    })) {
      ctx.contribute(COMMAND, command);
    }
  },
});

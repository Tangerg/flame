import { type ThemeAccess } from "@/features/theme/public/theme";
import { type AgentAccess } from "@/features/agent/public/agent";
import { type WorkspaceAccess } from "@/features/workspace/public/workspace";
import { type ComposerAccess } from "@/features/composer/public/composer";
import { type RuntimeAccess } from "@/features/connection/public/runtime";
import { type WorkingDirectoryAccess } from "@/features/navigation/public/workingDirectory";
import { type Navigator } from "@/lib/navigation";
import { type CommandSpec } from "@/features/command/public/commands";
import { toggleThemeScheme } from "@/features/theme/public/scheme";

import {
  closeActiveWorkspaceDockView,
  closeActiveWorkspaceView,
  toggleWorkspaceDock,
  toggleWorkspaceSidebar,
} from "@/features/workspace/public/navigation";
import { COMMAND } from "@/features/command/public/extensionPoints";
import { definePlugin } from "@/extensions/definePlugin";

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

function closeFocusedSurface(agentAccess: AgentAccess, workspaceAccess: WorkspaceAccess): void {
  if (closeActiveWorkspaceView(workspaceAccess)) return;
  if (closeActiveWorkspaceDockView(workspaceAccess)) return;
  agentAccess.closeActiveAgentSession();
}

export const defaultCommands = (
  agentAccess: AgentAccess,
  composerAccess: ComposerAccess,
  workspaceAccess: WorkspaceAccess,
  runtimeAccess: RuntimeAccess,
  directoryAccess: WorkingDirectoryAccess,
  navigation: Navigator,
  themeAccess: ThemeAccess,
) =>
  definePlugin({
    name: "flame.builtin.default-commands",
    setup(ctx) {
      for (const command of defaultStaticCommands({
        toggleSidebar: toggleWorkspaceSidebar.bind(null, workspaceAccess),
        toggleDock: toggleWorkspaceDock.bind(null, workspaceAccess),
        toggleTheme: () => toggleThemeScheme(themeAccess),
        newChat: createNewSession.bind(
          null,
          agentAccess,
          composerAccess,
          runtimeAccess,
          directoryAccess,
        ),
        closeFocused: closeFocusedSurface.bind(null, agentAccess, workspaceAccess),
        historyBack: () => navigation.back(),
        historyForward: () => navigation.forward(),
        previousSession: () => agentAccess.stepActiveAgentSession(-1),
        nextSession: () => agentAccess.stepActiveAgentSession(1),
      })) {
        ctx.contribute(COMMAND, command);
      }
    },
  });

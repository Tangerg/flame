import { type WorkspaceAccess } from "@/features/workspace/public/workspace";
import { createRef, type RefObject } from "react";
import { ChatPanel, type ChatPanelHandle } from "@/features/shell/workbench/panel/ChatPanel";
import { useWarmMarkdownRenderer } from "@/features/chat/public/message/rendering";
import { SettingsPage } from "@/features/settings/public/SettingsPage";
import { SidebarPanel } from "@/features/sidebar/public/SidebarPanel";
import { useChatSend } from "@/features/agent/public/input";
import { useReconcilePersistedAgentSessions } from "@/features/agent/public/session";
import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { COMMAND } from "@/features/command/public/extensionPoints";
import { WORKSPACE_VIEW } from "@/features/workspace/public/views";
import { WORKSPACE_SETTINGS_VIEW, openWorkspaceView } from "@/features/workspace/public/navigation";
import { useDefaultChatSession } from "@/features/agent/public/defaultSession";
import { ComposerProjectTray } from "@/features/shell/workbench/panel/ProjectSelector";
function WorkbenchChat({ panel }: { panel: RefObject<ChatPanelHandle | null> }) {
  useWarmMarkdownRenderer();
  useReconcilePersistedAgentSessions();
  useDefaultChatSession();
  const send = useChatSend();
  return <ChatPanel ref={panel} onSend={send} />;
}

function WorkbenchSidebar() {
  return <SidebarPanel />;
}

export const workbenchChat = definePlugin({
  name: "flame.builtin.workbench-chat",
  setup(ctx) {
    const panel = createRef<ChatPanelHandle>();
    function InstalledWorkbenchChat() {
      return <WorkbenchChat panel={panel} />;
    }
    contributeLayout(ctx, "app.main", { id: "chat", order: 0, component: InstalledWorkbenchChat });
    contributeLayout(ctx, "composer.overlay.top", {
      id: "project",
      order: -10,
      component: ComposerProjectTray,
    });
    ctx.contribute(COMMAND, {
      id: "composer.focus",
      label: "command.focusComposer",
      combo: "Mod+L",
      run: () => panel.current?.focusComposer(),
    });
  },
});

export const workbenchSidebar = definePlugin({
  name: "flame.builtin.workbench-sidebar",
  setup(ctx) {
    contributeLayout(ctx, "app.sidebar", { id: "sidebar", order: 0, component: WorkbenchSidebar });
  },
});

export const workbenchSettings = (workspaceAccess: WorkspaceAccess) =>
  definePlugin({
    name: "flame.builtin.workbench-settings",
    setup(ctx) {
      ctx.contribute(WORKSPACE_VIEW, {
        id: WORKSPACE_SETTINGS_VIEW,
        title: "settings.title",
        icon: "settings",
        order: 200,
        component: SettingsPage,
      });
      ctx.contribute(COMMAND, {
        id: "settings.open",
        label: "settings.title",
        combo: "Mod+,",
        run: () => openWorkspaceView(workspaceAccess, WORKSPACE_SETTINGS_VIEW),
      });
    },
  });

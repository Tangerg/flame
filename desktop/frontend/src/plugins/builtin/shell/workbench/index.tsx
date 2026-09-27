import { ChatPanel } from "./panel";
import { useWarmMarkdownRenderer } from "@/plugins/builtin/chat/message/public/rendering";
import { SettingsPage } from "./SettingsPage";
import { SidebarPanel } from "@/plugins/builtin/sidebar/public/SidebarPanel";
import { useChatSend } from "@/plugins/builtin/agent/public/input";
import { useReconcilePersistedAgentSessions } from "@/plugins/builtin/agent/public/session";
import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { COMMAND, WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import {
  WORKSPACE_SETTINGS_VIEW,
  openWorkspaceView,
} from "@/plugins/builtin/workspace/public/navigation";
import { useDefaultChatSession } from "@/plugins/builtin/agent/public/defaultSession";
import { ComposerProjectTray } from "./panel/ProjectSelector";

function WorkbenchChat() {
  useWarmMarkdownRenderer();
  useReconcilePersistedAgentSessions();
  useDefaultChatSession();
  const send = useChatSend();
  return <ChatPanel onSend={send} />;
}

function WorkbenchSidebar() {
  return <SidebarPanel />;
}

export const workbenchChat = definePlugin({
  name: "flame.builtin.workbench-chat",
  setup(ctx) {
    contributeLayout(ctx, "app.main", { id: "chat", order: 0, component: WorkbenchChat });
    contributeLayout(ctx, "composer.overlay.top", {
      id: "project",
      order: -10,
      component: ComposerProjectTray,
    });
  },
});

export const workbenchSidebar = definePlugin({
  name: "flame.builtin.workbench-sidebar",
  setup(ctx) {
    contributeLayout(ctx, "app.sidebar", { id: "sidebar", order: 0, component: WorkbenchSidebar });
  },
});

export const workbenchSettings = definePlugin({
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
      run: () => openWorkspaceView(WORKSPACE_SETTINGS_VIEW),
    });
  },
});

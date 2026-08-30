import { ChatPanel } from "./panel";
import { SettingsPage } from "./SettingsPage";
import { SidebarPanel } from "@/plugins/builtin/sidebar/public/SidebarPanel";
import { useSendComposerInput } from "@/plugins/builtin/chat/composer/public/sendToAgent";
import { useReconcilePersistedAgentSessions } from "@/plugins/builtin/agent/public/session";
import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import { useDefaultChatSession } from "@/plugins/builtin/agent/public/defaultSession";
import { ComposerProjectTray } from "./panel/ProjectSelector";

function KernelChat() {
  useReconcilePersistedAgentSessions();
  useDefaultChatSession();
  const send = useSendComposerInput();
  return <ChatPanel onSend={send} />;
}

function KernelSidebar() {
  return <SidebarPanel />;
}

export const kernelChat = definePlugin({
  name: "flame.builtin.kernel-chat",
  setup(ctx) {
    contributeLayout(ctx, "app.main", { id: "chat", order: 0, component: KernelChat });
    contributeLayout(ctx, "composer.overlay.top", {
      id: "project",
      order: -10,
      component: ComposerProjectTray,
    });
  },
});

export const kernelSidebar = definePlugin({
  name: "flame.builtin.kernel-sidebar",
  setup(ctx) {
    contributeLayout(ctx, "app.sidebar", { id: "sidebar", order: 0, component: KernelSidebar });
  },
});

export const kernelSettings = definePlugin({
  name: "flame.builtin.kernel-settings",
  setup(ctx) {
    ctx.contribute(WORKSPACE_VIEW, {
      id: "settings",
      title: "settings.title",
      icon: "settings",
      order: 200,
      component: SettingsPage,
    });
  },
});

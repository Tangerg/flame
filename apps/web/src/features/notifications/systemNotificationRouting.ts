import { type AgentAccess } from "@/features/agent/public/agent";
import { type WorkspaceAccess } from "@/features/workspace/public/workspace";
import { selectWorkspaceChat } from "@/features/workspace/public/navigation";
import { type SystemNotifier } from "@/features/notifications/systemNotifier";
import { type NotificationOwner } from "@/features/notifications/adapters/notifications";
import { definePlugin } from "@/extensions/definePlugin";
import { READY_HANDLER } from "@/extensions/lifecycle";

export function startSystemNotificationRouting(
  notifier: SystemNotifier,
  agent: AgentAccess,
  workspace: WorkspaceAccess,
  revealWindow: () => Promise<void>,
): () => void {
  const stop = notifier.onSystemNotificationOpened((sessionId) => {
    void revealWindow().catch((error: unknown) =>
      console.error("[notify] reveal window failed:", error),
    );
    agent.selectAgentSession(sessionId);
    selectWorkspaceChat(workspace);
  });
  void notifier
    .refreshNotificationAuthorization()
    .catch((error: unknown) =>
      console.error("[notify] notification authorization check failed:", error),
    );
  return stop;
}

export function createSystemNotificationsPlugin(notifications: NotificationOwner) {
  return definePlugin({
    name: "flame.builtin.system-notifications",
    setup(scope) {
      scope.cleanup(notifications.dispose);
      scope.contribute(READY_HANDLER, () => notifications.start());
    },
  });
}

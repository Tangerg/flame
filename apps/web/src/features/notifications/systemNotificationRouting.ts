import { type AgentAccess } from "@/features/agent/public/agent";
import { type SystemNotifier } from "@/features/notifications/systemNotifier";
import { type NotificationOwner } from "@/features/notifications/adapters/notifications";
import { definePlugin } from "@/extensions/definePlugin";
import { READY_HANDLER } from "@/extensions/lifecycle";

export const SYSTEM_NOTIFICATIONS_PLUGIN = "flame.builtin.system-notifications";

export function startSystemNotificationRouting(
  notifier: SystemNotifier,
  agent: Pick<AgentAccess, "openExistingAgentSession">,
  revealWindow: () => Promise<void>,
): () => void {
  const lifetime = new AbortController();
  const stop = notifier.onSystemNotificationOpened((sessionId) => {
    void revealWindow().catch((error: unknown) =>
      console.error("[notify] reveal window failed:", error),
    );
    void agent.openExistingAgentSession(sessionId, lifetime.signal).catch((error: unknown) => {
      if (!lifetime.signal.aborted)
        console.error("[notify] open notification Session failed", error);
    });
  });
  void notifier
    .refreshNotificationAuthorization()
    .catch((error: unknown) =>
      console.error("[notify] notification authorization check failed:", error),
    );
  return () => {
    stop();
    lifetime.abort();
  };
}

export function createSystemNotificationsPlugin(notifications: NotificationOwner) {
  return definePlugin({
    name: SYSTEM_NOTIFICATIONS_PLUGIN,
    setup(scope) {
      scope.lifetime.defer(notifications.dispose);
      scope.contributions.add(READY_HANDLER, () => notifications.start());
    },
  });
}

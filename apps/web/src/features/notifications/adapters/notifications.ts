import { type ClientHost } from "@/platform/host";
import { type AgentAccess } from "@/features/agent/public/agent";
import { type WorkspaceAccess } from "@/features/workspace/public/workspace";
import { createNotificationCentre } from "@/features/notifications/adapters/systemNotifier";
import { createSystemNotifier } from "@/features/notifications/systemNotifier";
import { createCompletionSoundStore } from "@/features/notifications/completionSound";
import { createSystemNotificationsStore } from "@/features/notifications/systemNotifications";
import { startCompletionNotifications } from "@/features/notifications/completionNotify";
import { type NotificationAccess } from "@/features/notifications/public/notifier";
export function createNotifications(
  host: Pick<
    ClientHost,
    | "notificationAuthorization"
    | "requestNotificationAuthorization"
    | "sendNotification"
    | "onNotificationOpened"
    | "revealWindow"
  >,
  agent: AgentAccess,
  workspace: WorkspaceAccess,
  productName: string,
) {
  const notifier = createSystemNotifier(createNotificationCentre(host));
  const access: NotificationAccess = {
    notifier,
    completionSound: createCompletionSoundStore(),
    systemNotifications: createSystemNotificationsStore(),
  };
  let stop: (() => void) | null = null;
  let disposed = false;
  return {
    access,
    start() {
      if (disposed) throw new Error("Notifications are disposed");
      if (!stop)
        stop = startCompletionNotifications(
          access,
          agent,
          workspace,
          () => host.revealWindow(),
          productName,
        );
    },
    dispose() {
      if (disposed) return;
      disposed = true;
      stop?.();
      stop = null;
      notifier.dispose();
    },
  };
}
export type NotificationOwner = ReturnType<typeof createNotifications>;

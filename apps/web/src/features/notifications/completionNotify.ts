import { type AgentAccess } from "@/features/agent/public/agent";
import { type WorkspaceAccess } from "@/features/workspace/public/workspace";
import { playCompletionChime } from "@/features/notifications/chime";
import { t } from "@/lib/i18n/index";
import { type RootRunSettlement, subscribeRootRunSettlements } from "@/features/agent/public/run";
import { selectAgentSession } from "@/features/agent/public/session";
import { selectWorkspaceChat } from "@/features/workspace/public/navigation";
import { type NotificationAccess } from "@/features/notifications/public/notifier";
import { type NotificationOwner } from "@/features/notifications/adapters/notifications";
import { definePlugin } from "@/extensions/definePlugin";
import { READY_HANDLER } from "@/extensions/lifecycle";
const SETTLEMENT_COPY = {
  finished: ["notify.finished.title", "notify.finished.body"],
  needsInput: ["notify.needsInput.title", "notify.needsInput.body"],
  error: ["notify.error.title", "notify.error.body"],
  canceled: ["notify.canceled.title", "notify.canceled.body"],
} as const satisfies Record<RootRunSettlement["status"], readonly [string, string]>;

function openSettledSession(
  agentAccess: AgentAccess,
  workspaceAccess: WorkspaceAccess,
  sessionId: string,
  revealWindow: () => Promise<void>,
): void {
  void revealWindow().catch((error: unknown) =>
    console.error("[notify] reveal window failed:", error),
  );
  selectAgentSession(agentAccess, sessionId);
  selectWorkspaceChat(workspaceAccess);
}

export async function announceSettlement(
  notifications: NotificationAccess,
  { sessionId, status, errorMessage }: RootRunSettlement,
  productName: string,
): Promise<void> {
  if (document.hasFocus()) return;
  if (notifications.completionSound.getState().completionSound) playCompletionChime();
  if (!notifications.systemNotifications.getState().systemNotifications) return;
  if ((await notifications.notifier.requestNotificationAuthorization()) !== "granted") return;
  const [titleKey, bodyKey] = SETTLEMENT_COPY[status];
  await notifications.notifier.sendSystemNotification({
    id: `run:${sessionId}`,
    title: t(titleKey, { product: productName }),
    body: status === "error" && errorMessage ? errorMessage : t(bodyKey),
    target: sessionId,
  });
}

export function startCompletionNotifications(
  notifications: NotificationAccess,
  agentAccess: AgentAccess,
  workspaceAccess: WorkspaceAccess,
  revealWindow: () => Promise<void>,
  productName: string,
): () => void {
  const unsubscribe = subscribeRootRunSettlements(
    agentAccess,
    (settlement) =>
      void announceSettlement(notifications, settlement, productName).catch((error: unknown) =>
        console.error("[notify] system notification failed:", error),
      ),
  );
  const stopOpening = notifications.notifier.onSystemNotificationOpened((sessionId) =>
    openSettledSession(agentAccess, workspaceAccess, sessionId, revealWindow),
  );
  void notifications.notifier
    .refreshNotificationAuthorization()
    .catch((error: unknown) =>
      console.error("[notify] notification authorization check failed:", error),
    );
  return () => {
    unsubscribe();
    stopOpening();
  };
}

export function createCompletionNotifyPlugin(notifications: NotificationOwner) {
  return definePlugin({
    name: "flame.builtin.completion-notify",
    setup(ctx) {
      ctx.cleanup(notifications.dispose);
      ctx.contribute(READY_HANDLER, () => notifications.start());
    },
  });
}

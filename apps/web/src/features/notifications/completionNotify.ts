import { type ClientHost } from "@/platform/host";
import { playCompletionChime } from "@/features/notifications/chime";
import { t } from "@/lib/i18n/index";
import {
  onSystemNotificationOpened,
  refreshNotificationAuthorization,
  requestNotificationAuthorization,
  sendSystemNotification,
} from "@/features/notifications/systemNotifier";
import { type RootRunSettlement, subscribeRootRunSettlements } from "@/features/agent/public/run";
import { selectAgentSession } from "@/features/agent/public/session";
import { selectWorkspaceChat } from "@/features/workspace/public/navigation";
import { installNotificationCentre } from "@/features/notifications/adapters/systemNotifier";
import { definePlugin } from "@/extensions/definePlugin";
import { READY_HANDLER } from "@/extensions/lifecycle";
import { useCompletionSoundStore } from "@/features/notifications/completionSound";
import { useSystemNotificationsStore } from "@/features/notifications/systemNotifications";
const SETTLEMENT_COPY = {
  finished: ["notify.finished.title", "notify.finished.body"],
  needsInput: ["notify.needsInput.title", "notify.needsInput.body"],
  error: ["notify.error.title", "notify.error.body"],
  canceled: ["notify.canceled.title", "notify.canceled.body"],
} as const satisfies Record<RootRunSettlement["status"], readonly [string, string]>;

function openSettledSession(sessionId: string, revealWindow: () => Promise<void>): void {
  void revealWindow().catch((error: unknown) =>
    console.error("[notify] reveal window failed:", error),
  );
  selectAgentSession(sessionId);
  selectWorkspaceChat();
}

export async function announceSettlement(
  { sessionId, status, errorMessage }: RootRunSettlement,
  productName: string,
): Promise<void> {
  if (document.hasFocus()) return;
  if (useCompletionSoundStore.getState().completionSound) playCompletionChime();
  if (!useSystemNotificationsStore.getState().systemNotifications) return;
  if ((await requestNotificationAuthorization()) !== "granted") return;
  const [titleKey, bodyKey] = SETTLEMENT_COPY[status];
  await sendSystemNotification({
    id: `run:${sessionId}`,
    title: t(titleKey, { product: productName }),
    body: status === "error" && errorMessage ? errorMessage : t(bodyKey),
    target: sessionId,
  });
}

export function startCompletionNotifications(
  revealWindow: () => Promise<void>,
  productName: string,
): () => void {
  const unsubscribe = subscribeRootRunSettlements(
    (settlement) =>
      void announceSettlement(settlement, productName).catch((error: unknown) =>
        console.error("[notify] system notification failed:", error),
      ),
  );
  const stopOpening = onSystemNotificationOpened((sessionId) =>
    openSettledSession(sessionId, revealWindow),
  );
  void refreshNotificationAuthorization().catch((error: unknown) =>
    console.error("[notify] notification authorization check failed:", error),
  );
  return () => {
    unsubscribe();
    stopOpening();
  };
}

export function createCompletionNotifyPlugin(
  host: Pick<
    ClientHost,
    | "notificationAuthorization"
    | "requestNotificationAuthorization"
    | "sendNotification"
    | "onNotificationOpened"
    | "revealWindow"
  >,
  productName: string,
) {
  return definePlugin({
    name: "flame.builtin.completion-notify",
    setup(ctx) {
      ctx.cleanup(installNotificationCentre(host));
      ctx.contribute(READY_HANDLER, () => {
        ctx.cleanup(startCompletionNotifications(() => host.revealWindow(), productName));
      });
    },
  });
}

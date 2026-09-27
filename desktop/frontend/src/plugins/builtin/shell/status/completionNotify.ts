import type { ClientHost } from "@/platform/host";
import { playCompletionChime } from "./chime";
import { t } from "@/lib/i18n";
import {
  onSystemNotificationOpened,
  refreshNotificationAuthorization,
  requestNotificationAuthorization,
  sendSystemNotification,
} from "./systemNotifier";
import {
  type RootRunSettlement,
  subscribeRootRunSettlements,
} from "@/plugins/builtin/agent/public/run";
import { selectAgentSession } from "@/plugins/builtin/agent/public/session";
import { selectWorkspaceChat } from "@/plugins/builtin/workspace/public/navigation";
import { installNotificationCentre } from "./adapters/systemNotifier";
import { definePlugin, READY_HANDLER } from "@/plugins/sdk";
import { useCompletionSoundStore } from "./completionSound";
import { useSystemNotificationsStore } from "./systemNotifications";
import { PRODUCT_NAME } from "@/product";

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

export async function announceSettlement({
  sessionId,
  status,
  errorMessage,
}: RootRunSettlement): Promise<void> {
  if (document.hasFocus()) return;
  if (useCompletionSoundStore.getState().completionSound) playCompletionChime();
  if (!useSystemNotificationsStore.getState().systemNotifications) return;
  if ((await requestNotificationAuthorization()) !== "granted") return;
  const [titleKey, bodyKey] = SETTLEMENT_COPY[status];
  await sendSystemNotification({
    id: `run:${sessionId}`,
    title: t(titleKey, { product: PRODUCT_NAME }),
    body: status === "error" && errorMessage ? errorMessage : t(bodyKey),
    target: sessionId,
  });
}

export function startCompletionNotifications(revealWindow: () => Promise<void>): () => void {
  const unsubscribe = subscribeRootRunSettlements(
    (settlement) =>
      void announceSettlement(settlement).catch((error: unknown) =>
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
) {
  return definePlugin({
    name: "flame.builtin.completion-notify",
    setup(ctx) {
      ctx.cleanup(installNotificationCentre(host));
      ctx.contribute(READY_HANDLER, () => {
        ctx.cleanup(startCompletionNotifications(() => host.revealWindow()));
      });
    },
  });
}

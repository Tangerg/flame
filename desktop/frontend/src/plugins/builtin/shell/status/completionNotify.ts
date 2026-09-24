import { playCompletionChime } from "./chime";
import { disposeOnHmr } from "@/lib/hmr";
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
import { revealDesktopWindow } from "./adapters/desktopWindow";
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

function openSettledSession(sessionId: string): void {
  void revealDesktopWindow().catch((error: unknown) =>
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

export function startCompletionNotifications(): () => void {
  const unsubscribe = subscribeRootRunSettlements(
    (settlement) =>
      void announceSettlement(settlement).catch((error: unknown) =>
        console.error("[notify] system notification failed:", error),
      ),
  );
  const stopOpening = onSystemNotificationOpened(openSettledSession);
  void refreshNotificationAuthorization().catch((error: unknown) =>
    console.error("[notify] notification authorization check failed:", error),
  );
  return () => {
    unsubscribe();
    stopOpening();
  };
}

export const completionNotify = definePlugin({
  name: "flame.builtin.completion-notify",
  setup(ctx) {
    const uninstall = installNotificationCentre();
    ctx.cleanup(uninstall);
    let stop: (() => void) | undefined;
    ctx.contribute(READY_HANDLER, () => {
      stop = startCompletionNotifications();
      disposeOnHmr(stop);
    });
    ctx.cleanup(() => stop?.());
  },
});

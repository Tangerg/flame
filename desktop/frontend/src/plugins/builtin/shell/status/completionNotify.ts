import { playCompletionChime } from "./chime";
import { disposeOnHmr } from "@/lib/hmr";
import { ensureOsNotifyPermission, osNotify } from "./osNotify";
import {
  type RootRunSettlement,
  subscribeRootRunSettlements,
} from "@/plugins/builtin/agent/public/run";
import { definePlugin, READY_HANDLER } from "@/plugins/sdk";
import { useCompletionSoundStore } from "./completionSound";
import { PRODUCT_NAME } from "@/product";

function onSettled({ sessionId, status, errorMessage }: RootRunSettlement): void {
  if (document.hasFocus()) return;

  let title = `${PRODUCT_NAME} finished`;
  let body = "The agent finished its turn.";
  switch (status) {
    case "needsInput":
      title = `${PRODUCT_NAME} needs your input`;
      body = "The agent is waiting for your approval or answer.";
      break;
    case "error":
      title = `${PRODUCT_NAME} hit an error`;
      body = errorMessage ?? "The agent run failed.";
      break;
    case "canceled":
      title = `${PRODUCT_NAME} stopped`;
      body = "The agent run was canceled.";
      break;
    case "finished":
      break;
  }
  osNotify(title, { body, tag: `run:${sessionId}` });
  if (useCompletionSoundStore.getState().completionSound) playCompletionChime();
}

export const completionNotify = definePlugin({
  name: "flame.builtin.completion-notify",
  setup(ctx) {
    ensureOsNotifyPermission();
    let unsubscribe: (() => void) | undefined;
    ctx.contribute(READY_HANDLER, () => {
      unsubscribe = subscribeRootRunSettlements(onSettled);
      disposeOnHmr(unsubscribe);
    });
    ctx.cleanup(() => unsubscribe?.());
  },
});

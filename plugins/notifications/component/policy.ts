import {
  createTranslator,
  type ComponentContext,
  type RootRunSettlement,
} from "@flame/component-sdk";
import { catalogs } from "./catalogs";
import { playCompletionChime } from "./chime";
import { readPreferences } from "./preferences";

const copy = {
  finished: ["notify.finished.title", "notify.finished.body"],
  needsInput: ["notify.needsInput.title", "notify.needsInput.body"],
  error: ["notify.error.title", "notify.error.body"],
  canceled: ["notify.canceled.title", "notify.canceled.body"],
} as const satisfies Record<
  RootRunSettlement["status"],
  readonly [string, string]
>;

export async function announceSettlement(
  context: ComponentContext,
  settlement: RootRunSettlement,
  signal: AbortSignal,
): Promise<void> {
  signal.throwIfAborted();
  if (document.hasFocus()) return;
  const preferences = readPreferences();
  const sound = preferences.completionSound
    ? context.track(() => playCompletionChime(signal))
    : Promise.resolve();
  const notify = async () => {
    if (!preferences.systemNotifications) return;
    if ((await context.notifications.requestAuthorization()) !== "granted")
      return;
    signal.throwIfAborted();
    const t = createTranslator(catalogs, context.locale);
    const [title, body] = copy[settlement.status];
    await context.notifications.send({
      id: `run:${settlement.sessionId}`,
      sessionId: settlement.sessionId,
      title: t(title, { product: context.notifications.applicationName }),
      body:
        settlement.status === "error" && settlement.errorMessage
          ? settlement.errorMessage
          : t(body),
    });
  };
  await Promise.all([sound, notify()]);
}

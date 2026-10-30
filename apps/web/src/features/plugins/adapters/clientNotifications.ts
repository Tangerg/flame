import {
  type ComponentContext,
  type ComponentNotification,
  type ComponentReference,
  type SystemNotificationChannel,
} from "@flame/component-sdk";
import { type ContributionLifetime } from "@/extensions/definePlugin";
import { trackContributionIO } from "@/extensions/contributionIO";
import { reportPluginError } from "@/extensions/errors";
import { type AgentAccess } from "@/features/agent/public/agent";
import { type NotificationAccess } from "@/features/notifications/public/notifier";

function ownSubscription(
  signal: AbortSignal,
  start: (active: () => boolean) => () => void,
): () => void {
  signal.throwIfAborted();
  let active = true;
  const unsubscribe = start(() => active && !signal.aborted);
  const stop = () => {
    if (!active) return;
    active = false;
    signal.removeEventListener("abort", stop);
    unsubscribe();
  };
  signal.addEventListener("abort", stop, { once: true });
  return stop;
}

export function createComponentNotifications(
  lifetime: ContributionLifetime,
  signal: AbortSignal,
  component: ComponentReference,
  access: NotificationAccess,
): SystemNotificationChannel {
  const notifier = access.notifier;
  const run = <T>(operation: () => Promise<T>): Promise<T> => {
    signal.throwIfAborted();
    return trackContributionIO(lifetime, async () => {
      const result = await operation();
      signal.throwIfAborted();
      return result;
    });
  };
  return Object.freeze({
    applicationName: access.productName,
    authorization: () => notifier.useNotificationAuthorization.getState().authorization,
    subscribe: (listener: () => void) =>
      ownSubscription(signal, (active) =>
        notifier.useNotificationAuthorization.subscribe(() => {
          if (!active()) return;
          try {
            listener();
          } catch (error) {
            reportPluginError(`package:${component.installationId}`, "events", error);
          }
        }),
      ),
    refreshAuthorization: () => run(notifier.refreshNotificationAuthorization),
    requestAuthorization: () => run(notifier.requestNotificationAuthorization),
    send: (notification: ComponentNotification) =>
      run(() =>
        notifier.sendSystemNotification({
          id: JSON.stringify([component.installationId, component.componentId, notification.id]),
          title: notification.title,
          body: notification.body,
          target: notification.sessionId,
        }),
      ),
  });
}

export function createComponentSettlementSubscription(
  lifetime: ContributionLifetime,
  signal: AbortSignal,
  component: ComponentReference,
  subscribe: AgentAccess["subscribeRootRunSettlements"],
): ComponentContext["subscribeRootRunSettlements"] {
  return (listener) =>
    ownSubscription(signal, (active) =>
      subscribe((settlement) => {
        if (!active()) return;
        const work = trackContributionIO(lifetime, async () => {
          signal.throwIfAborted();
          await listener(Object.freeze({ ...settlement }), signal);
        });
        void work.catch((error: unknown) => {
          if (!signal.aborted)
            reportPluginError(`package:${component.installationId}`, "events", error);
        });
      }),
    );
}

import { create } from "zustand";
import {
  type NotificationAuthorization,
  type SystemNotification,
  notificationCentre,
} from "./application/ports/notificationCentre";

export const useNotificationAuthorization = create<{
  authorization: NotificationAuthorization;
}>()(() => ({ authorization: "default" }));

function settle(authorization: NotificationAuthorization): NotificationAuthorization {
  useNotificationAuthorization.setState({ authorization });
  return authorization;
}

export async function refreshNotificationAuthorization(): Promise<NotificationAuthorization> {
  return settle(await notificationCentre().authorization());
}

export async function requestNotificationAuthorization(): Promise<NotificationAuthorization> {
  return settle(await notificationCentre().requestAuthorization());
}

export function sendSystemNotification(notification: SystemNotification): Promise<boolean> {
  return notificationCentre().send(notification);
}

export function onSystemNotificationOpened(open: (target: string) => void): () => void {
  return notificationCentre().onOpened(open);
}

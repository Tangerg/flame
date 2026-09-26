import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { NotificationAuthorization } from "@/foundation/notificationAuthorization";

export type { NotificationAuthorization } from "@/foundation/notificationAuthorization";

export interface SystemNotification {
  id: string;
  title: string;
  body: string;
  target: string;
}

// The platform's notification centre. `target` is opaque to it and comes back
// through onOpened when the user clicks the notification.
export interface NotificationCentre {
  authorization(): Promise<NotificationAuthorization>;
  requestAuthorization(): Promise<NotificationAuthorization>;
  send(notification: SystemNotification): Promise<boolean>;
  onOpened(open: (target: string) => void): () => void;
}

const port = createSingletonPort<NotificationCentre>("Notification centre is not configured");

export const configureNotificationCentre = port.configure;
export const notificationCentre = port.get;

import { type NotificationAuthorization } from "@flame/component-sdk/notifications";

export interface SystemNotification {
  id: string;
  title: string;
  body: string;
  target: string;
}

export interface NotificationCentre {
  authorization(): Promise<NotificationAuthorization>;
  requestAuthorization(): Promise<NotificationAuthorization>;
  send(notification: SystemNotification): Promise<boolean>;
  onOpened(open: (target: string) => void): () => void;
}

import type { PluginCarrier } from "@/foundation/pluginCarrier";
import type { NotificationAuthorization } from "@/foundation/notificationAuthorization";

interface RuntimeTarget {
  endpoint: string;
  localToken?: string;
}

export interface ClientBootstrap {
  runtime: RuntimeTarget;
  localFilesystemEndpoint: string | null;
}

interface ClientNotification {
  id: string;
  title: string;
  body?: string;
  target?: string;
}

export interface ClientHost {
  readonly kind: "desktop" | "web";
  readonly pluginCarrier: PluginCarrier;
  bootstrap(): Promise<ClientBootstrap>;
  chooseWorkingDirectory(): Promise<string | null>;
  saveImage(source: string): Promise<boolean>;
  windowChrome(): Promise<{ controlsCentreY: number; controlsInlineEnd: number } | null>;
  revealWindow(): Promise<void>;
  revealPath(path: string): Promise<boolean>;
  openPath(path: string): Promise<boolean>;
  notificationAuthorization(): Promise<NotificationAuthorization>;
  requestNotificationAuthorization(): Promise<NotificationAuthorization>;
  sendNotification(notification: ClientNotification): Promise<boolean>;
  onNotificationOpened(listener: (target: string) => void): Promise<() => void>;
}

import { type NotificationAuthorization } from "@/foundation/notificationAuthorization";
import { type FileSaveResult, type SaveFileRequest } from "@flame/component-sdk";
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
  bootstrap(): Promise<ClientBootstrap>;
  chooseWorkingDirectory(): Promise<string | null>;
  // Capture bytes before yielding. Native resolves after writing or cancellation;
  // Browser reports only download dispatch. Only Browser consumes the MIME type.
  saveFile(request: SaveFileRequest): Promise<FileSaveResult>;
  windowChrome(): Promise<{ controlsCentreY: number; controlsInlineEnd: number } | null>;
  revealWindow(): Promise<void>;
  revealPath(path: string): Promise<boolean>;
  openPath(path: string): Promise<boolean>;
  notificationAuthorization(): Promise<NotificationAuthorization>;
  requestNotificationAuthorization(): Promise<NotificationAuthorization>;
  sendNotification(notification: ClientNotification): Promise<boolean>;
  onNotificationOpened(listener: (target: string) => void): Promise<() => void>;
}

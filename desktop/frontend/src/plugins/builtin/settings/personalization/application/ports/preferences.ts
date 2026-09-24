import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { StreamReveal } from "@/plugins/builtin/chat/message/public/streamReveal";
import type { NotificationPermissionState } from "@/plugins/builtin/shell/status/public/notifications";

interface PersonalizationPreferencesPort {
  useCompletionSound(): boolean;
  useSetCompletionSound(): (on: boolean) => void;
  useSystemNotifications(): boolean;
  useSetSystemNotifications(): (on: boolean) => void;
  useNotificationPermission(): NotificationPermissionState;
  requestNotificationPermission(): Promise<NotificationPermissionState>;
  useStreamReveal(): StreamReveal;
  useSetStreamReveal(): (mode: StreamReveal) => void;
}

const port = createSingletonPort<PersonalizationPreferencesPort>(
  "Personalization preferences port is not configured",
);

export const configurePersonalizationPreferencesPort = port.configure;
export const personalizationPreferences = port.get;

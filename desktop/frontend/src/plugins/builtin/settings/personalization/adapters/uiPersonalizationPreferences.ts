import { useStreamRevealStore } from "@/plugins/builtin/chat/message/public/streamReveal";
import { useCompletionSoundStore } from "@/plugins/builtin/shell/status/public/completionSound";
import {
  requestNotificationPermission,
  useNotificationPermission,
  useSystemNotificationsStore,
} from "@/plugins/builtin/shell/status/public/notifications";
import { configurePersonalizationPreferencesPort } from "../application/ports/preferences";

export function installPersonalizationPreferencesPort(): () => void {
  return configurePersonalizationPreferencesPort({
    useCompletionSound: () => useCompletionSoundStore((state) => state.completionSound),
    useSetCompletionSound: () => useCompletionSoundStore((state) => state.setCompletionSound),
    useSystemNotifications: () => useSystemNotificationsStore((state) => state.systemNotifications),
    useSetSystemNotifications: () =>
      useSystemNotificationsStore((state) => state.setSystemNotifications),
    useNotificationPermission: () => useNotificationPermission((state) => state.permission),
    requestNotificationPermission,
    useStreamReveal: () => useStreamRevealStore((state) => state.streamReveal),
    useSetStreamReveal: () => useStreamRevealStore((state) => state.setStreamReveal),
  });
}

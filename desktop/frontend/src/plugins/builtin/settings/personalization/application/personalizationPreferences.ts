import { personalizationPreferences } from "./ports/preferences";

export function useCompletionSoundPreference() {
  return {
    completionSound: personalizationPreferences().useCompletionSound(),
    setCompletionSound: personalizationPreferences().useSetCompletionSound(),
  };
}

export function useSystemNotificationsPreference() {
  const port = personalizationPreferences();
  return {
    systemNotifications: port.useSystemNotifications(),
    setSystemNotifications: port.useSetSystemNotifications(),
    permission: port.useNotificationPermission(),
    requestPermission: port.requestNotificationPermission,
  };
}

export function useStreamRevealPreference() {
  return {
    streamReveal: personalizationPreferences().useStreamReveal(),
    setStreamReveal: personalizationPreferences().useSetStreamReveal(),
  };
}

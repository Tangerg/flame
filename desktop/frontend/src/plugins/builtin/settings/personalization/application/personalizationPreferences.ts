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
    authorization: port.useNotificationAuthorization(),
    refreshAuthorization: port.refreshNotificationAuthorization,
    requestAuthorization: port.requestNotificationAuthorization,
  };
}

export function useStreamRevealPreference() {
  return {
    streamReveal: personalizationPreferences().useStreamReveal(),
    setStreamReveal: personalizationPreferences().useSetStreamReveal(),
  };
}

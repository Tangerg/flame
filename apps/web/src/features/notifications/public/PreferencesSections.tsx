import { useT } from "@/lib/i18n/index";
import { SettingRow } from "@/ui/settings/SettingRow";
import { useEffect } from "react";
import { Switch } from "@flame/ui";
import { useNotifications } from "@/features/notifications/public/notifier";
export function CompletionSoundSection() {
  const { completionSound: useCompletionSoundStore } = useNotifications();
  const t = useT();
  const completionSound = useCompletionSoundStore((state) => state.completionSound);
  const setCompletionSound = useCompletionSoundStore((state) => state.setCompletionSound);

  return (
    <SettingRow label={t("settings.completionSound")} sub={t("settings.completionSound.sub")}>
      <Switch
        checked={completionSound}
        onCheckedChange={setCompletionSound}
        ariaLabel={t("settings.completionSound")}
      />
    </SettingRow>
  );
}

export function SystemNotificationsSection() {
  const { systemNotifications: useSystemNotificationsStore, notifier } = useNotifications();
  const {
    useNotificationAuthorization,
    refreshNotificationAuthorization,
    requestNotificationAuthorization,
  } = notifier;
  const t = useT();
  const systemNotifications = useSystemNotificationsStore((state) => state.systemNotifications);
  const setSystemNotifications = useSystemNotificationsStore(
    (state) => state.setSystemNotifications,
  );
  const authorization = useNotificationAuthorization((state) => state.authorization);
  const refreshAuthorization = refreshNotificationAuthorization;
  const requestAuthorization = requestNotificationAuthorization;
  useEffect(() => {
    void refreshAuthorization().catch((error: unknown) =>
      console.error("[settings] notification authorization check failed:", error),
    );
  }, [refreshAuthorization]);
  const blocked = authorization === "denied" || authorization === "unsupported";
  return (
    <SettingRow
      label={t("settings.notifications")}
      sub={t(`settings.notifications.authorization.${authorization}`)}
    >
      <Switch
        checked={systemNotifications && !blocked}
        disabled={blocked}
        ariaLabel={t("settings.notifications")}
        onCheckedChange={(on) => {
          setSystemNotifications(on);
          if (on) void requestAuthorization();
        }}
      />
    </SettingRow>
  );
}

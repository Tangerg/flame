import { useT } from "@/lib/i18n/index";
import { SettingRow } from "@/ui/settings/SettingRow";
import { useEffect } from "react";
import { Switch } from "@flame/ui";
import { Checkbox } from "@flame/ui";
import { useCompletionSoundStore } from "@/features/notifications/public/completionSound";
import {
  refreshNotificationAuthorization,
  requestNotificationAuthorization,
  useNotificationAuthorization,
  useSystemNotificationsStore,
} from "@/features/notifications/public/notifications";
export function CompletionSoundSection() {
  const t = useT();
  const completionSound = useCompletionSoundStore((state) => state.completionSound);
  const setCompletionSound = useCompletionSoundStore((state) => state.setCompletionSound);

  return (
    <SettingRow label={t("settings.completionSound")} sub={t("settings.completionSound.sub")}>
      <Checkbox
        checked={completionSound}
        onCheckedChange={setCompletionSound}
        label={t("settings.completionSound.toggle")}
      />
    </SettingRow>
  );
}

export function SystemNotificationsSection() {
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

import { useEffect } from "react";
import { Checkbox, Segmented, Switch } from "@/ui";
import { useT } from "@/lib/i18n";
import {
  useCompletionSoundPreference,
  useStreamRevealPreference,
  useSystemNotificationsPreference,
} from "../application/personalizationPreferences";
import { SettingRow } from "../../kit";

export function CompletionSoundSection() {
  const t = useT();
  const { completionSound, setCompletionSound } = useCompletionSoundPreference();

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
  const {
    systemNotifications,
    setSystemNotifications,
    authorization,
    refreshAuthorization,
    requestAuthorization,
  } = useSystemNotificationsPreference();
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

export function StreamRevealSection() {
  const t = useT();
  const { streamReveal, setStreamReveal } = useStreamRevealPreference();

  return (
    <SettingRow label={t("settings.streamReveal")} sub={t("settings.streamReveal.sub")}>
      <Segmented
        value={streamReveal}
        options={[
          { value: "smooth", label: t("settings.streamReveal.smooth") },
          { value: "typewriter", label: t("settings.streamReveal.typewriter") },
        ]}
        onChange={setStreamReveal}
        ariaLabel={t("settings.streamReveal")}
      />
    </SettingRow>
  );
}

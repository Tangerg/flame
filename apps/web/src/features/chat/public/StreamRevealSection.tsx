import { useT } from "@/lib/i18n/index";
import { SettingRow } from "@/ui/settings/SettingRow";
import { Segmented } from "@flame/ui";
import { useStreamRevealStore } from "@/features/chat/public/message/streamReveal";
export function StreamRevealSection() {
  const t = useT();
  const streamReveal = useStreamRevealStore((state) => state.streamReveal);
  const setStreamReveal = useStreamRevealStore((state) => state.setStreamReveal);

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

// A preference rather than a constant because it is a taste axis — Material ships the same
// thing as a scheme variant rather than picking one chroma and defending it. `standard` is
// what every surface here was measured against, so the default changes nothing.

import type { SegmentedOption } from "@/ui";
import { Segmented } from "@/ui";
import type { AccentTint } from "@/lib/appearance";
import { ACCENT_TINTS } from "@/lib/appearance";
import { useT } from "@/lib/i18n";
import { useAccentTintPreference } from "../application/appearancePreferences";
import { SettingRow } from "../../public";

export function AccentTintSection() {
  const t = useT();
  const { accentTint, setAccentTint } = useAccentTintPreference();

  const options: SegmentedOption<AccentTint>[] = ACCENT_TINTS.map((tint) => ({
    value: tint,
    label: t(`settings.accentTint.${tint}`),
  }));

  return (
    <SettingRow label={t("settings.accentTint")} sub={t("settings.accentTint.sub")}>
      <Segmented
        value={accentTint}
        options={options}
        onChange={setAccentTint}
        ariaLabel={t("settings.accentTint")}
      />
    </SettingRow>
  );
}

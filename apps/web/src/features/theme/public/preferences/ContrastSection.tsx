import * as stylex from "@stylexjs/stylex";
import { Slider } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import { useAppearance, useAppearanceEdit } from "@/features/theme/public/appearance";
import { SettingRow } from "@/ui/settings/SettingRow";
import { type as typeStep } from "@flame/ui/tokens.stylex";
import { settingStyles as ss } from "@/ui/settings/settingStyles";

export function ContrastSection() {
  const t = useT();
  const contrast = useAppearance((s) => s.contrast);
  const { setContrast } = useAppearanceEdit();

  return (
    <SettingRow label={t("settings.contrast")}>
      <div {...stylex.props(ss.lineWide)}>
        <Slider
          value={contrast}
          min={0}
          max={100}
          onValueChange={setContrast}
          ariaLabel={t("settings.contrast")}
        />
        <span {...stylex.props(ss.readout, typeStep.uiMd)}>{contrast}</span>
      </div>
    </SettingRow>
  );
}

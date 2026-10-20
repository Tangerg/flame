import * as stylex from "@stylexjs/stylex";
import { Slider } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import { useAppearance, editAppearance } from "@/features/theme/public/appearance";
import { SettingRow } from "@/ui/settings/SettingRow";
import { color, space, type as typeStep } from "@flame/ui/tokens.stylex";
import { settingStyles as ss } from "@/ui/settings/settingStyles";
const a = stylex.create({
  readout: {
    width: space.s7,
    textAlign: "right",
    fontFamily: "var(--font-mono)",
    color: color.fgMuted,
  },
});

export function ContrastSection() {
  const t = useT();
  const contrast = useAppearance((s) => s.contrast);
  const { setContrast } = editAppearance();

  return (
    <SettingRow label={t("settings.contrast")} sub={t("settings.contrast.sub")} align="start">
      <div {...stylex.props(ss.lineWide)}>
        <Slider
          value={contrast}
          min={0}
          max={100}
          onValueChange={setContrast}
          ariaLabel={t("settings.contrast")}
        />
        <span {...stylex.props(a.readout, typeStep.uiMd)}>{contrast}</span>
      </div>
    </SettingRow>
  );
}

import * as stylex from "@stylexjs/stylex";
import { Slider } from "@/ui";
import { useT } from "@/lib/i18n";
import { useContrastPreference } from "../application/appearancePreferences";
import { SettingRow } from "../../kit";
import { color, space, type as typeStep } from "@/styles/tokens.stylex";
import { settingStyles as ss } from "../../kit/settingStyles";

const a = stylex.create({
  // A fixed measure so the number does not shift the slider as it counts up.
  readout: {
    width: space.s7,
    textAlign: "right",
    fontFamily: "var(--font-mono)",
    color: color.fgMuted,
  },
});

export function ContrastSection() {
  const t = useT();
  const { contrast, setContrast } = useContrastPreference();

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

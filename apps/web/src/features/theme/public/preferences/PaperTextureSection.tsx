import * as stylex from "@stylexjs/stylex";
import { Segmented, Slider, type SegmentedOption } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import {
  PAPER_TEXTURES,
  useAppearance,
  editAppearance,
  type PaperTexture,
} from "@/features/theme/public/appearance";
import { SettingRow } from "@/ui/settings/SettingRow";
import { type as typeStep } from "@flame/ui/tokens.stylex";
import { settingStyles as ss } from "@/ui/settings/settingStyles";

const TEXTURE_OPTIONS: SegmentedOption<PaperTexture>[] = PAPER_TEXTURES.map((texture) => ({
  value: texture,
  label: `settings.paperTexture.${texture}`,
}));

export function PaperTextureSection() {
  const t = useT();
  const paperTexture = useAppearance((s) => s.paperTexture);
  const strength = useAppearance((s) => s.paperTextureStrength);
  const { setPaperTexture, setPaperTextureStrength } = editAppearance();

  return (
    <>
      <SettingRow label={t("settings.paperTexture")}>
        <Segmented
          value={paperTexture}
          options={TEXTURE_OPTIONS.map((o) => ({ ...o, label: t(o.label) }))}
          onChange={setPaperTexture}
          ariaLabel={t("settings.paperTexture")}
        />
      </SettingRow>
      {paperTexture !== "off" && (
        <SettingRow label={t("settings.paperTexture.strength")}>
          <div {...stylex.props(ss.lineWide)}>
            <Slider
              value={strength}
              min={1}
              max={5}
              step={1}
              onValueChange={setPaperTextureStrength}
              ariaLabel={t("settings.paperTexture.strength")}
            />
            <span {...stylex.props(ss.readout, typeStep.uiMd)}>{strength}</span>
          </div>
        </SettingRow>
      )}
    </>
  );
}

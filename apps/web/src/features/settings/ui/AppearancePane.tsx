import { PanelSection } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import { SettingsGroup } from "@/ui/settings/SettingsGroup";
import { AccentSection } from "@/features/theme/public/preferences/AccentSection";
import { ContrastSection } from "@/features/theme/public/preferences/ContrastSection";
import { CustomThemeColors } from "@/features/theme/public/preferences/CustomThemeColors";
import { FontSection } from "@/features/theme/public/preferences/FontSection";
import { LanguageSection } from "@/features/i18n/public/LanguageSection";
import { ShapeMotionSection } from "@/features/theme/public/preferences/ShapeMotionSection";
import { ThemeSection } from "@/features/theme/public/preferences/ThemeSection";
export function AppearancePane() {
  const t = useT();
  return (
    <>
      <PanelSection title={t("settings.appearance.colors")}>
        <SettingsGroup>
          <ThemeSection />
          <CustomThemeColors />
          <AccentSection />
          <ContrastSection />
        </SettingsGroup>
      </PanelSection>
      <PanelSection title={t("settings.appearance.typography")}>
        <SettingsGroup>
          <FontSection />
        </SettingsGroup>
      </PanelSection>
      <PanelSection title={t("settings.appearance.interaction")}>
        <SettingsGroup>
          <ShapeMotionSection />
        </SettingsGroup>
      </PanelSection>
      <PanelSection title={t("settings.appearance.language")}>
        <SettingsGroup>
          <LanguageSection />
        </SettingsGroup>
      </PanelSection>
    </>
  );
}

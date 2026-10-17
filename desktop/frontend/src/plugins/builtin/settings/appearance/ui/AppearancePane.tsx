import { PanelSection } from "@flame/ui";
import { useT } from "@/lib/i18n";
import { SettingsGroup } from "../../kit";
import { AccentSection } from "./AccentSection";
import { ContrastSection } from "./ContrastSection";
import { CustomThemeColors } from "./CustomThemeColors";
import { FontSection } from "./FontSection";
import { LanguageSection } from "./LanguageSection";
import { ShapeMotionSection } from "./ShapeMotionSection";
import { ThemeSection } from "./ThemeSection";

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

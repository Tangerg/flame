import { SettingsGroup } from "../../kit";
import { AccentSection } from "./AccentSection";
import { AccentTintSection } from "./AccentTintSection";
import { ContrastSection } from "./ContrastSection";
import { CustomThemeColors } from "./CustomThemeColors";
import { FontSection } from "./FontSection";
import { LanguageSection } from "./LanguageSection";
import { ShapeMotionSection } from "./ShapeMotionSection";
import { ThemeSection } from "./ThemeSection";

export function AppearancePane() {
  return (
    <SettingsGroup>
      <ThemeSection />
      <CustomThemeColors />
      <AccentSection />
      <AccentTintSection />
      <ContrastSection />
      <FontSection />
      <ShapeMotionSection />
      <LanguageSection />
    </SettingsGroup>
  );
}

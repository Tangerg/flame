import { type AppearanceProjection } from "@/lib/appearance";
import { type AppearancePreferencePort } from "@/features/theme/application/appearancePreference";
import { type SystemAppearance } from "@/features/theme/application/systemAppearance";
import { type FontAvailability } from "@/features/theme/application/preferences/systemFonts";
export interface ThemeAccess {
  preference: AppearancePreferencePort;
  system: SystemAppearance;
  fonts: FontAvailability;
  appearance: AppearanceProjection;
}

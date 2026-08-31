import type { AppearanceEdit, AppearancePreference } from "../kit/appearance";
import { appearancePreferencePort } from "./ports/appearancePreference";

export function useAppearance<T>(select: (preference: AppearancePreference) => T): T {
  return appearancePreferencePort().use(select);
}

export function editAppearance(): AppearanceEdit {
  return appearancePreferencePort().edit();
}

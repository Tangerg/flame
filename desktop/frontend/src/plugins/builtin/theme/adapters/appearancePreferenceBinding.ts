import { configureAppearancePreferencePort } from "../application/ports/appearancePreference";
import { useAppearanceStore } from "./appearanceStore";

export function installAppearancePreferencePort(): () => void {
  return configureAppearancePreferencePort({
    use: (select) => useAppearanceStore(select),
    read: () => useAppearanceStore.getState(),
    edit: () => useAppearanceStore.getState(),
  });
}

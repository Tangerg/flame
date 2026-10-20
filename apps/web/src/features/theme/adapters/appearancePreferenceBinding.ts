import { configureAppearancePreferencePort } from "@/features/theme/application/ports/appearancePreference";
import { useAppearanceStore } from "@/features/theme/adapters/appearanceStore";
export function installAppearancePreferencePort(): () => void {
  return configureAppearancePreferencePort({
    use: (select) => useAppearanceStore(select),
    read: () => useAppearanceStore.getState(),
    edit: () => useAppearanceStore.getState(),
    subscribe: (listener) => useAppearanceStore.subscribe(listener),
  });
}

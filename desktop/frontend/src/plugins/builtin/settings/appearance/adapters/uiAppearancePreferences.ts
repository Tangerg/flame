// Binds this pane's port to the theme context's published preference. The pane declares
// what it needs field by field; the theme publishes one selector — the translation between
// the two is exactly what an adapter is for.

import { editAppearance, useAppearance } from "@/plugins/builtin/theme/public/appearance";
import { configureAppearancePreferencesPort } from "../application/ports/preferences";

export function installAppearancePreferencesPort(): () => void {
  return configureAppearancePreferencesPort({
    useTheme: () => useAppearance((state) => state.theme),
    useSetTheme: () => editAppearance().setTheme,
    useAccent: () => useAppearance((state) => state.accent),
    useSetAccent: () => editAppearance().setAccent,
    useCustomTheme: () => useAppearance((state) => state.customTheme),
    useSetCustomTheme: () => editAppearance().setCustomTheme,
    useContrast: () => useAppearance((state) => state.contrast),
    useAccentTint: () => useAppearance((state) => state.accentTint),
    useSetAccentTint: () => editAppearance().setAccentTint,
    useSetContrast: () => editAppearance().setContrast,
    useUiFont: () => useAppearance((state) => state.uiFont),
    useCodeFont: () => useAppearance((state) => state.codeFont),
    useFontSize: () => useAppearance((state) => state.fontSize),
    useFontSmoothing: () => useAppearance((state) => state.fontSmoothing),
    useSetUiFont: () => editAppearance().setUiFont,
    useSetCodeFont: () => editAppearance().setCodeFont,
    useSetFontSize: () => editAppearance().setFontSize,
    useSetFontSmoothing: () => editAppearance().setFontSmoothing,
    useRadiusScale: () => useAppearance((state) => state.radiusScale),
    useMotionScale: () => useAppearance((state) => state.motionScale),
    useDensity: () => useAppearance((state) => state.density),
    useSetDensity: () => editAppearance().setDensity,
    useSetRadiusScale: () => editAppearance().setRadiusScale,
    useSetMotionScale: () => editAppearance().setMotionScale,
  });
}

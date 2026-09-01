// Grouped the way the pane's sections read them. There is no port between here and the
// theme context: this pane exists to edit that context's preference, it already imports
// that context's vocabulary, and a port nothing ever substitutes is indirection, not
// inversion — it restated the same thirteen fields a third and fourth time.

import { resolveThemeScheme } from "@/plugins/builtin/theme/public/scheme";
import { editAppearance, useAppearance } from "@/plugins/builtin/theme/public/appearance";

export function useThemePreference() {
  return { theme: useAppearance((s) => s.theme), setTheme: editAppearance().setTheme };
}

export function useAccentPreference() {
  return {
    accent: useAppearance((s) => s.accent),
    setAccent: editAppearance().setAccent,
    scheme: resolveThemeScheme(useAppearance((s) => s.theme)),
  };
}

export function useCustomThemePreference() {
  return {
    theme: useAppearance((s) => s.theme),
    customTheme: useAppearance((s) => s.customTheme),
    setCustomTheme: editAppearance().setCustomTheme,
  };
}

export function useContrastPreference() {
  return {
    contrast: useAppearance((s) => s.contrast),
    setContrast: editAppearance().setContrast,
  };
}

export function useAccentTintPreference() {
  return {
    accentTint: useAppearance((s) => s.accentTint),
    setAccentTint: editAppearance().setAccentTint,
  };
}

export function useFontPreferences() {
  const edit = editAppearance();
  return {
    uiFont: useAppearance((s) => s.uiFont),
    codeFont: useAppearance((s) => s.codeFont),
    fontSize: useAppearance((s) => s.fontSize),
    fontSmoothing: useAppearance((s) => s.fontSmoothing),
    setUiFont: edit.setUiFont,
    setCodeFont: edit.setCodeFont,
    setFontSize: edit.setFontSize,
    setFontSmoothing: edit.setFontSmoothing,
  };
}

export function useShapeMotionPreferences() {
  const edit = editAppearance();
  return {
    density: useAppearance((s) => s.density),
    radiusScale: useAppearance((s) => s.radiusScale),
    motionScale: useAppearance((s) => s.motionScale),
    setDensity: edit.setDensity,
    setRadiusScale: edit.setRadiusScale,
    setMotionScale: edit.setMotionScale,
  };
}

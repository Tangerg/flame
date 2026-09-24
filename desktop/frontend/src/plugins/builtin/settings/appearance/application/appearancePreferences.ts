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

export function useFontPreferences() {
  const edit = editAppearance();
  return {
    uiFont: useAppearance((s) => s.uiFont),
    codeFont: useAppearance((s) => s.codeFont),
    fontSize: useAppearance((s) => s.fontSize),
    codeFontSize: useAppearance((s) => s.codeFontSize),
    fontSmoothing: useAppearance((s) => s.fontSmoothing),
    setUiFont: edit.setUiFont,
    setCodeFont: edit.setCodeFont,
    setFontSize: edit.setFontSize,
    setCodeFontSize: edit.setCodeFontSize,
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

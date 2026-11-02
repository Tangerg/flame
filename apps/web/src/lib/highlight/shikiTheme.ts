import { type ThemeRegistration } from "shiki/core";
import githubDark from "shiki/themes/github-dark.mjs";
import githubLightHighContrast from "shiki/themes/github-light-high-contrast.mjs";
import { type Scheme } from "@/lib/appearance";

export const SHIKI_THEME: Record<Scheme, ThemeRegistration> = {
  dark: githubDark,
  light: githubLightHighContrast,
};

export function shikiThemeName(scheme: Scheme): string {
  return SHIKI_THEME[scheme].name!;
}

export function syntaxColorToken(color: string): string {
  return `color-code-${color.slice(1).toLowerCase()}`;
}

export function syntaxColors(): string[] {
  const colors = Object.values(SHIKI_THEME).flatMap((theme) => [
    theme.fg ?? theme.colors?.["editor.foreground"],
    ...(theme.tokenColors ?? []).map((rule) => rule.settings.foreground),
  ]);
  return [
    ...new Set(
      colors
        .filter((color): color is string => color !== undefined)
        .map((color) => color.toLowerCase()),
    ),
  ];
}

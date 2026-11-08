import * as stylex from "@stylexjs/stylex";
import type { CSSProperties } from "react";
import { color, surface } from "@flame/ui/tokens.stylex";
import { defaultAppearance } from "@/features/theme/kit/appearance";
import { customPalette } from "@/features/theme/kit/customPalette";
import { paletteThemeContribution, type PaletteTheme } from "@/features/theme/kit/palette";
import { uiTypeLadderCssVariables } from "@/features/theme/kit/typeLadder";
import { failureSelection, fixtureColors, recoveryURL } from "./selection";

export const FAILURE_TITLE_ID = "lab-failure-title";

export const failureStyles = stylex.create({
  root: {
    boxSizing: "border-box",
    height: "100dvh",
    padding: 24,
    overflow: "auto",
    overflowWrap: "anywhere",
    backgroundColor: surface.canvas,
    color: color.fg,
    fontFamily: "var(--font-sans)",
    fontSize: "var(--fs-ui-md)",
    lineHeight: 1.5,
  },
  heading: {
    margin: 0,
    fontSize: "var(--fs-display-sm)",
    fontWeight: 600,
    lineHeight: 1.4,
  },
  detail: {
    maxWidth: "80ch",
    marginTop: 12,
    marginBottom: 20,
    whiteSpace: "pre-wrap",
    overflowWrap: "anywhere",
    fontFamily: "var(--font-mono)",
    fontSize: "var(--fs-code)",
    lineHeight: 1.6,
  },
  recovery: {
    display: "inline-flex",
    alignItems: "center",
    minHeight: 24,
    maxWidth: "100%",
    color: color.accentText,
    textDecoration: "underline",
    textUnderlineOffset: "0.2em",
  },
});

// Resolve semantic aliases beside the scoped request variables, rather than at the document root.
const scopedColor = stylex.createTheme(color, {
  fg: "var(--color-text)",
  accentText: "var(--color-accent-text)",
});
const scopedSurface = stylex.createTheme(surface, {
  canvas: "var(--color-bg)",
});
export const failureRootProps = stylex.props(failureStyles.root, scopedColor, scopedSurface);

export function failurePresentation(query: URLSearchParams, context: "lab" | "fixture" = "lab") {
  const selection = failureSelection(query, context);
  const preference = {
    ...defaultAppearance(),
    theme: selection.theme,
    ...(context === "fixture" ? fixtureColors(query) : {}),
  };
  const palette: PaletteTheme =
    preference.theme === "custom"
      ? customPalette(preference.customTheme)
      : {
          id: selection.theme,
          label: selection.theme,
          scheme: selection.theme,
          palette: {},
        };
  const contribution = paletteThemeContribution(
    palette,
    preference.accent,
    preference.contrast,
    preference,
  );
  const tokens = contribution.tokens;
  if (tokens === undefined) throw new Error("lab failure palette has no color tokens");
  // Fatal readers project request appearance locally; the document painter keeps its owner.
  const variables: Record<string, string> = Object.fromEntries(
    Object.entries(tokens).map(([name, value]) => [`--${name}`, value]),
  );
  Object.assign(variables, uiTypeLadderCssVariables(selection.fontSize), {
    "--motion-scale": selection.motion === "off" ? "0" : "1",
  });
  const style: CSSProperties = { ...variables, colorScheme: palette.scheme };
  return {
    locale: selection.locale,
    href: recoveryURL({ ...selection, theme: palette.scheme }),
    variables,
    scheme: palette.scheme,
    style,
  };
}
